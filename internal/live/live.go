// Package live builds a small, temporary database for one PIN straight from
// the Cook County open data API, so a home can be checked without
// downloading the whole county.
//
// It fetches only the rows the comparison engine reads for one home: the
// home's assessed values, every assessed value and parcel in its Assessor
// neighborhood, and the addresses, characteristics and Board of Review
// appeals of the PIN blocks that neighborhood covers. Rows load through the
// same code as a full build (store.LoadRaw, then ingest.Build) into an
// in-memory SQLite database, so comps.Engine runs on it unchanged.
package live

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/ingest"
	"github.com/assessgap/assessgap/internal/socrata"
	"github.com/assessgap/assessgap/internal/store"
	_ "modernc.org/sqlite"
)

// maxBlocksPerQuery keeps request URLs to a few kilobytes.
const maxBlocksPerQuery = 150

// Stats describes the API traffic for one check.
type Stats struct {
	Calls    int
	Bytes    int64
	Duration time.Duration
	Blocks   int // 7-digit PIN blocks fetched for the neighborhood
}

// Fetcher builds per-PIN databases from the API.
type Fetcher struct {
	Domain   string
	AppToken string
	HTTP     *http.Client // nil means a client with a 60 s timeout
	Log      func(string, ...any)
}

// Result is a temporary database holding one PIN's comparison inputs.
type Result struct {
	DB    *sql.DB
	Stats Stats
}

// Close releases the in-memory database.
func (r *Result) Close() error { return r.DB.Close() }

// ErrNotFound means the API has no assessed value for the PIN.
var ErrNotFound = comps.ErrNotFound

// counter counts requests and response bytes through an http.RoundTripper.
type counter struct {
	next         http.RoundTripper
	calls, bytes atomic.Int64
}

func (c *counter) RoundTrip(r *http.Request) (*http.Response, error) {
	c.calls.Add(1)
	resp, err := c.next.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	resp.Body = &countingBody{ReadCloser: resp.Body, n: &c.bytes}
	return resp, nil
}

type countingBody struct {
	io.ReadCloser
	n *atomic.Int64
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.n.Add(int64(n))
	return n, err
}

// Fetch builds the database for pin. year 0 means the latest year with an
// assessed value, as in comps.Engine.Report.
func (f *Fetcher) Fetch(ctx context.Context, pin string, year int) (*Result, error) {
	start := time.Now()
	logf := f.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}
	base := http.DefaultTransport
	timeout := 60 * time.Second
	if f.HTTP != nil {
		if f.HTTP.Transport != nil {
			base = f.HTTP.Transport
		}
		timeout = f.HTTP.Timeout
	}
	cnt := &counter{next: base}
	client := &socrata.Client{Domain: f.Domain, AppToken: f.AppToken, PageSize: 50000,
		HTTP: &http.Client{Transport: cnt, Timeout: timeout}, Log: logf}

	tmp, err := os.MkdirTemp("", "assessgap-live-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one connection, so the in-memory database persists
	db.SetConnMaxIdleTime(0)
	ok := false
	defer func() {
		if !ok {
			db.Close()
		}
	}()

	q := &queries{client: client, db: db, dir: tmp, log: logf}
	quote := sqlQuote(pin)

	// Round 1: the subject's assessed values give its year and neighborhood.
	if err := q.load(ctx, "assessed_values", "pin = "+quote); err != nil {
		return nil, err
	}
	y, nbhd, err := subjectYear(ctx, db, pin, year)
	if err != nil {
		return nil, err
	}

	// Round 2: everything assessed in the neighborhood that year, and the
	// neighborhood's parcels (for addresses and appeal statistics).
	g, gctx := newGroup(ctx)
	g.Go(func() error {
		return q.fetch(gctx, "assessed_values", fmt.Sprintf("year = '%d' AND nbhd = %s", y, sqlQuote(nbhd)))
	})
	g.Go(func() error {
		return q.fetch(gctx, "parcel_universe", fmt.Sprintf("nbhd_code = %s OR pin = %s", sqlQuote(nbhd), quote))
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	if err := q.loadFetched("assessed_values", "parcel_universe"); err != nil {
		return nil, err
	}
	blocks, err := pinBlocks(ctx, db)
	if err != nil {
		return nil, err
	}

	// Round 3: the per-PIN datasets, by PIN block, plus the subject's own rows.
	g, gctx = newGroup(ctx)
	for _, name := range []string{"addresses", "res_chars", "bor_appeals"} {
		for i, chunk := range chunks(blocks, maxBlocksPerQuery) {
			name, part, where := name, fmt.Sprintf("%s-%d", name, i), blockWhere(chunk)
			g.Go(func() error { return q.fetchAs(gctx, name, part, where) })
		}
	}
	for _, name := range []string{"sales", "condo_chars", "ccao_appeals"} {
		name := name
		g.Go(func() error { return q.fetch(gctx, name, "pin = "+quote) })
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	if err := q.loadFetched("addresses", "res_chars", "bor_appeals", "sales", "condo_chars", "ccao_appeals"); err != nil {
		return nil, err
	}
	if err := ingest.Build(ctx, db, logf); err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	ok = true
	return &Result{DB: db, Stats: Stats{
		Calls: int(cnt.calls.Load()), Bytes: cnt.bytes.Load(), Duration: time.Since(start), Blocks: len(blocks),
	}}, nil
}

// queries fetches dataset pages into dir/<part>/ and loads them as raw tables.
type queries struct {
	client *socrata.Client
	db     *sql.DB
	dir    string
	log    func(string, ...any)

	mu    sync.Mutex
	parts map[string][]string // dataset name -> fetched part dirs
}

func dataset(name, where string) (socrata.Dataset, error) {
	for _, d := range ingest.Datasets {
		if d.Name == name {
			d.Where = ingest.And(d.Where, where)
			d.Select = strings.Join(ingest.Columns[name], ", ")
			return d, nil
		}
	}
	return socrata.Dataset{}, fmt.Errorf("live: unknown dataset %q", name)
}

func (q *queries) fetch(ctx context.Context, name, where string) error {
	return q.fetchAs(ctx, name, name+"-x", where)
}

func (q *queries) fetchAs(ctx context.Context, name, part, where string) error {
	d, err := dataset(name, where)
	if err != nil {
		return err
	}
	dir := filepath.Join(q.dir, part)
	if _, err := q.client.Fetch(ctx, d, dir); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.parts == nil {
		q.parts = map[string][]string{}
	}
	q.parts[name] = append(q.parts[name], dir)
	return nil
}

// load fetches and immediately loads one dataset (round 1).
func (q *queries) load(ctx context.Context, name, where string) error {
	if err := q.fetchAs(ctx, name, name+"-0", where); err != nil {
		return err
	}
	return q.loadFetched(name)
}

// loadFetched (re)creates raw_<name> from every part fetched so far, merging
// the pages of all parts. A dataset with no rows gets an empty table.
func (q *queries) loadFetched(names ...string) error {
	for _, name := range names {
		merged := filepath.Join(q.dir, "merged-"+name)
		if err := os.MkdirAll(merged, 0o755); err != nil {
			return err
		}
		n := 0
		q.mu.Lock()
		parts := append([]string(nil), q.parts[name]...)
		q.mu.Unlock()
		sort.Strings(parts)
		for _, p := range parts {
			pages, _ := filepath.Glob(filepath.Join(p, "page-*.csv"))
			sort.Strings(pages)
			for _, pg := range pages {
				if err := os.Link(pg, filepath.Join(merged, fmt.Sprintf("page-%05d.csv", n))); err != nil {
					return err
				}
				n++
			}
		}
		if n == 0 {
			if err := emptyRaw(q.db, name); err != nil {
				return err
			}
		} else if _, err := store.LoadRaw(q.db, name, merged, q.log); err != nil {
			return err
		}
		if err := os.RemoveAll(merged); err != nil {
			return err
		}
	}
	return nil
}

func emptyRaw(db *sql.DB, name string) error {
	t := "raw_" + name
	_, err := db.Exec(fmt.Sprintf(`DROP TABLE IF EXISTS %s; CREATE TABLE %s ("%s")`, t, t, strings.Join(ingest.Columns[name], `", "`)))
	return err
}

// subjectYear picks the report year and the subject's neighborhood that year.
func subjectYear(ctx context.Context, db *sql.DB, pin string, year int) (int, string, error) {
	var y sql.NullInt64
	if year == 0 {
		err := db.QueryRowContext(ctx, `SELECT MAX(CAST(CAST(year AS REAL) AS INTEGER)) FROM raw_assessed_values
			WHERE pin = ? AND COALESCE(certified_tot, mailed_tot) IS NOT NULL`, pin).Scan(&y)
		if err != nil {
			return 0, "", err
		}
		if !y.Valid {
			return 0, "", ErrNotFound
		}
		year = int(y.Int64)
	}
	var nbhd sql.NullString
	err := db.QueryRowContext(ctx, `SELECT nbhd FROM raw_assessed_values
		WHERE pin = ? AND CAST(CAST(year AS REAL) AS INTEGER) = ?`, pin, year).Scan(&nbhd)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	if err != nil {
		return 0, "", err
	}
	if !nbhd.Valid || nbhd.String == "" {
		return 0, "", fmt.Errorf("live: PIN %s has no neighborhood for %d", pin, year)
	}
	// The code goes into the next SoQL query; Assessor neighborhoods are
	// always 5 digits (township + neighborhood), so refuse anything else.
	if len(nbhd.String) != 5 || !isDigits(nbhd.String) {
		return 0, "", fmt.Errorf("live: PIN %s has an unexpected neighborhood code %q for %d", pin, nbhd.String, year)
	}
	return year, nbhd.String, nil
}

// pinBlocks lists the 7-digit PIN blocks (area, subarea, block) of every
// parcel and assessment loaded so far.
func pinBlocks(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT substr(pin, 1, 7) FROM raw_assessed_values
		UNION SELECT substr(pin, 1, 7) FROM raw_parcel_universe
		ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		if isDigits(b) && len(b) == 7 {
			out = append(out, b)
		}
	}
	return out, rows.Err()
}

func blockWhere(blocks []string) string {
	parts := make([]string, len(blocks))
	for i, b := range blocks {
		parts[i] = "starts_with(pin, '" + b + "')"
	}
	return strings.Join(parts, " OR ")
}

func chunks(xs []string, n int) [][]string {
	var out [][]string
	for len(xs) > n {
		out = append(out, xs[:n])
		xs = xs[n:]
	}
	if len(xs) > 0 {
		out = append(out, xs)
	}
	return out
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

// group runs functions concurrently and keeps the first error, canceling
// the others' context when one fails.
type group struct {
	wg     sync.WaitGroup
	cancel context.CancelFunc
	once   sync.Once
	err    error
}

func newGroup(ctx context.Context) (*group, context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	return &group{cancel: cancel}, ctx
}

func (g *group) Go(f func() error) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		if err := f(); err != nil {
			g.once.Do(func() { g.err = err; g.cancel() })
		}
	}()
}

func (g *group) Wait() error {
	g.wg.Wait()
	g.cancel()
	return g.err
}

// sqlQuote quotes s as a SoQL string literal.
func sqlQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
