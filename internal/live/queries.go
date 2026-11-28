package live

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/assessgap/assessgap/internal/ingest"
	"github.com/assessgap/assessgap/internal/socrata"
	"github.com/assessgap/assessgap/internal/store"
)

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

// sqlQuote quotes s as a SoQL string literal.
func sqlQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
