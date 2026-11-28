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
//
// This file has the fetch plan, round by round; queries.go fetches and loads
// datasets, and group.go runs a round's requests concurrently.
package live

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/ingest"
	"github.com/assessgap/assessgap/internal/socrata"
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
	client, cnt := f.client(logf)

	tmp, err := os.MkdirTemp("", "assessgap-live-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	db, err := memDB()
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			db.Close()
		}
	}()

	q := &queries{client: client, db: db, dir: tmp, log: logf}
	y, nbhd, err := q.subjectRound(ctx, pin, year)
	if err != nil {
		return nil, err
	}
	blocks, err := q.neighborhoodRound(ctx, pin, y, nbhd)
	if err != nil {
		return nil, err
	}
	if err := q.blockRound(ctx, pin, blocks); err != nil {
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

// client is the Socrata client for one check, counting its traffic.
func (f *Fetcher) client(logf func(string, ...any)) (*socrata.Client, *counter) {
	base := http.DefaultTransport
	timeout := 60 * time.Second
	if f.HTTP != nil {
		if f.HTTP.Transport != nil {
			base = f.HTTP.Transport
		}
		timeout = f.HTTP.Timeout
	}
	cnt := &counter{next: base}
	return &socrata.Client{Domain: f.Domain, AppToken: f.AppToken, PageSize: 50000,
		HTTP: &http.Client{Transport: cnt, Timeout: timeout}, Log: logf}, cnt
}

// memDB opens an empty in-memory database.
func memDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one connection, so the in-memory database persists
	db.SetConnMaxIdleTime(0)
	return db, nil
}

// subjectRound is round 1: the subject's assessed values give its year and
// neighborhood.
func (q *queries) subjectRound(ctx context.Context, pin string, year int) (int, string, error) {
	if err := q.load(ctx, "assessed_values", "pin = "+sqlQuote(pin)); err != nil {
		return 0, "", err
	}
	return subjectYear(ctx, q.db, pin, year)
}

// neighborhoodRound is round 2: everything assessed in the neighborhood that
// year, and the neighborhood's parcels (for addresses and appeal statistics).
// It returns the PIN blocks they cover.
func (q *queries) neighborhoodRound(ctx context.Context, pin string, y int, nbhd string) ([]string, error) {
	g, gctx := newGroup(ctx)
	g.Go(func() error {
		return q.fetch(gctx, "assessed_values", fmt.Sprintf("year = '%d' AND nbhd = %s", y, sqlQuote(nbhd)))
	})
	g.Go(func() error {
		return q.fetch(gctx, "parcel_universe", fmt.Sprintf("nbhd_code = %s OR pin = %s", sqlQuote(nbhd), sqlQuote(pin)))
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	if err := q.loadFetched("assessed_values", "parcel_universe"); err != nil {
		return nil, err
	}
	return pinBlocks(ctx, q.db)
}

// blockRound is round 3: the per-PIN datasets, by PIN block, plus the
// subject's own rows.
func (q *queries) blockRound(ctx context.Context, pin string, blocks []string) error {
	g, gctx := newGroup(ctx)
	for _, name := range []string{"addresses", "res_chars", "bor_appeals"} {
		for i, chunk := range chunks(blocks, maxBlocksPerQuery) {
			name, part, where := name, fmt.Sprintf("%s-%d", name, i), blockWhere(chunk)
			g.Go(func() error { return q.fetchAs(gctx, name, part, where) })
		}
	}
	for _, name := range []string{"sales", "condo_chars", "ccao_appeals"} {
		name := name
		g.Go(func() error { return q.fetch(gctx, name, "pin = "+sqlQuote(pin)) })
	}
	if err := g.Wait(); err != nil {
		return err
	}
	return q.loadFetched("addresses", "res_chars", "bor_appeals", "sales", "condo_chars", "ccao_appeals")
}
