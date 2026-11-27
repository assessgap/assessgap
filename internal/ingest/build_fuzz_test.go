package ingest_test

import (
	"context"
	"encoding/csv"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/assessgap/assessgap/internal/ingest"
	"github.com/assessgap/assessgap/internal/store"
)

// decimal is the shape Socrata uses for numbers in CSV ("30000", "2026.0").
var decimal = regexp.MustCompile(`^[+-]?[0-9]+(\.[0-9]*)?([eE][+-]?[0-9]+)?$`)

// writeRaw writes one CSV page per dataset under dir, header only except for
// the rows given.
func writeRaw(t *testing.T, dir string, rows map[string][]string) {
	t.Helper()
	for _, d := range ingest.Datasets {
		sub := filepath.Join(dir, d.Name)
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(sub, "page-00000.csv"))
		if err != nil {
			t.Fatal(err)
		}
		w := csv.NewWriter(f)
		cols := ingest.Columns[d.Name]
		if err := w.Write(cols); err != nil {
			t.Fatal(err)
		}
		if r, ok := rows[d.Name]; ok {
			if err := w.Write(r); err != nil {
				t.Fatal(err)
			}
		}
		w.Flush()
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// FuzzBuildFields loads one parcel whose year, assessed value and building
// square footage are arbitrary text, as Socrata could send them, and runs
// build.sql. The build must succeed whatever the text, and a field written
// as a decimal number must come out as that number: these are the inputs of
// every AV-per-square-foot figure.
func FuzzBuildFields(f *testing.F) {
	for _, s := range [][3]string{
		{"2026", "30000", "1200"}, {"2026.0", "30000.0", "1200.0"}, {"2026", "3e4", "1.2e3"},
		{"", "", ""}, {"abc", "N/A", "-"}, {"2026", " 30000", "1,200"}, {"99999999999999999999", "1e400", "-0"},
		{"2026", "0x10", "Infinity"}, {"2026.9", "-5", "0"},
	} {
		f.Add(s[0], s[1], s[2])
	}
	f.Fuzz(func(t *testing.T, year, tot, sf string) {
		for _, s := range []string{year, tot, sf} {
			if strings.ContainsAny(s, "\x00\r") {
				return // not representable in a CSV field the way Socrata writes them
			}
		}
		dir := t.TempDir()
		const pin = "13331030130000"
		writeRaw(t, dir, map[string][]string{
			"parcel_universe": {pin, "2026", "203", "71", "Jefferson", "71280", "", "", "41.9", "-87.7"},
			"assessed_values": {pin, year, "203", "71280", tot, "", "", "", "", "", "", "", ""},
			"res_chars": {pin, "2026", "1", "203", sf, "3000", "1920", "3", "1", "0", "6", "", "1", "1", "1",
				"1", "", "", "0", "", ""},
		})
		db, err := store.Open(filepath.Join(dir, "cook.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		quiet := func(string, ...any) {}
		if err := ingest.Load(db, ingest.Datasets, dir, quiet); err != nil {
			t.Fatal(err)
		}
		if err := ingest.Build(context.Background(), db, quiet); err != nil {
			// A row with no year fails assessment's NOT NULL and stops the
			// build: loud, and a refresh keeps the old database. Anything
			// else must build. (docs/dev/2026-11-27-e2e.md, fuzz results.)
			if year == "" {
				return
			}
			t.Fatalf("build with year %q, total %q, bldg sf %q: %v", year, tot, sf, err)
		}
		check := func(field, text, query string) {
			if !decimal.MatchString(text) {
				return
			}
			want, err := strconv.ParseFloat(text, 64)
			if err != nil || math.IsInf(want, 0) {
				return
			}
			var got float64
			if err := db.QueryRow(query).Scan(&got); err != nil {
				t.Fatalf("%s %q: %v", field, text, err)
			}
			if got != want && math.Abs(got-want) > 1e-9*math.Abs(want) {
				t.Fatalf("%s %q became %v", field, text, got)
			}
		}
		check("certified total", tot, `SELECT mailed FROM assessment`)
		check("building sq ft", sf, `SELECT bldg_sf FROM res_char`)
		if decimal.MatchString(year) {
			if y, err := strconv.ParseFloat(year, 64); err == nil && math.Abs(y) < 1e15 {
				var got int64
				if err := db.QueryRow(`SELECT year FROM assessment`).Scan(&got); err != nil {
					t.Fatalf("year %q: %v", year, err)
				}
				if got != int64(y) {
					t.Fatalf("year %q became %d", year, got)
				}
			}
		}
	})
}
