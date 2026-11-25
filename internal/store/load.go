// Package store loads raw CSV pages into SQLite.
package store

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/assessgap/assessgap/internal/socrata"
	_ "modernc.org/sqlite"
)

// Open opens (or creates) a SQLite database tuned for bulk loading.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=synchronous(OFF)&_pragma=temp_store(MEMORY)&_pragma=cache_size(-1000000)&_pragma=busy_timeout(10000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, db.Ping()
}

var identRe = regexp.MustCompile(`[^a-z0-9_]`)

func ident(s string) string { return identRe.ReplaceAllString(strings.ToLower(s), "_") }

// LoadRaw replaces table raw_<name> with all rows from dir/page-*.csv.
// Columns are untyped (SQLite affinity); typed tables are built by SQL later.
// Empty strings become NULL.
func LoadRaw(db *sql.DB, name, dir string, log func(string, ...any)) (int, error) {
	pages, _ := filepath.Glob(filepath.Join(dir, "page-*.csv"))
	sort.Strings(pages)
	if len(pages) == 0 {
		return 0, fmt.Errorf("no pages in %s", dir)
	}
	header, err := readHeader(pages[0])
	if err != nil {
		return 0, err
	}
	table := "raw_" + ident(name)
	cols := make([]string, len(header))
	for i, h := range header {
		cols[i] = `"` + ident(h) + `"`
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(fmt.Sprintf(`DROP TABLE IF EXISTS %s; CREATE TABLE %s (%s)`, table, table, strings.Join(cols, ", "))); err != nil {
		return 0, err
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",")
	stmt, err := tx.Prepare(fmt.Sprintf(`INSERT INTO %s (%s) VALUES (%s)`, table, strings.Join(cols, ","), ph))
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	total := 0
	args := make([]any, len(cols))
	for _, p := range pages {
		f, err := os.Open(p)
		if err != nil {
			return total, err
		}
		r := socrata.NewCSVReader(bufio.NewReaderSize(f, 1<<20))
		h, err := r.Read()
		if err != nil {
			f.Close()
			return total, fmt.Errorf("%s: %w", p, err)
		}
		// map this page's header order onto the table's column order
		idx := make([]int, len(cols))
		pos := map[string]int{}
		for i, c := range h {
			pos[ident(c)] = i
		}
		for i, c := range header {
			j, ok := pos[ident(c)]
			if !ok {
				j = -1
			}
			idx[i] = j
		}
		for {
			rec, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				f.Close()
				return total, fmt.Errorf("%s: %w", p, err)
			}
			for i, j := range idx {
				if j < 0 || j >= len(rec) || rec[j] == "" {
					args[i] = nil
				} else {
					args[i] = rec[j]
				}
			}
			if _, err := stmt.Exec(args...); err != nil {
				f.Close()
				return total, err
			}
			total++
		}
		f.Close()
	}
	if err := tx.Commit(); err != nil {
		return total, err
	}
	log("%s: loaded %d rows into %s", name, total, table)
	return total, nil
}

func readHeader(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h, err := socrata.NewCSVReader(f).Read()
	if err != nil {
		return nil, err
	}
	return append([]string(nil), h...), nil
}
