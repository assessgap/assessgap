package refresh

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func full(n int64) map[string]int64 {
	m := map[string]int64{}
	for _, t := range Tables {
		m[t] = n
	}
	return m
}

func with(m map[string]int64, t string, n int64) map[string]int64 {
	out := map[string]int64{}
	for k, v := range m {
		out[k] = v
	}
	if n < 0 {
		delete(out, t)
	} else {
		out[t] = n
	}
	return out
}

func TestCheck(t *testing.T) {
	base := Snapshot{Counts: full(1000), LatestYear: 2026}
	tests := []struct {
		name string
		old  *Snapshot
		cur  Snapshot
		want []string // substrings, one per expected problem
	}{
		{"identical", &base, base, nil},
		{"growth is fine", &base, Snapshot{Counts: full(5000), LatestYear: 2027}, nil},
		{"2% shrink is allowed", &base, Snapshot{Counts: with(full(1000), "sale", 980), LatestYear: 2026}, nil},
		{"2.1% shrink fails", &base, Snapshot{Counts: with(full(1000), "sale", 979), LatestYear: 2026},
			[]string{"sale: shrank 2.1%"}},
		{"empty table", &base, Snapshot{Counts: with(full(1000), "parcel", 0), LatestYear: 2026},
			[]string{"parcel: table is empty"}},
		{"missing table", &base, Snapshot{Counts: with(full(1000), "res_char", -1), LatestYear: 2026},
			[]string{"res_char: table missing"}},
		{"year backwards", &base, Snapshot{Counts: full(1000), LatestYear: 2025},
			[]string{"latest year went backwards (2026 → 2025)"}},
		{"first build: only non-empty applies", nil, Snapshot{Counts: with(full(10), "sale", 0), LatestYear: 2020},
			[]string{"sale: table is empty"}},
		{"table new since old build", &Snapshot{Counts: with(full(1000), "sale", -1), LatestYear: 2026},
			Snapshot{Counts: full(1000), LatestYear: 2026}, nil},
		{"several problems", &base, Snapshot{Counts: with(with(full(1000), "parcel", 0), "sale", 500), LatestYear: 2024},
			[]string{"parcel: table is empty", "sale: shrank 50.0%", "went backwards"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Check(tt.old, tt.cur, 0.02)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d problems %q, want %d %q", len(got), got, len(tt.want), tt.want)
			}
			for i, w := range tt.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("problem %d = %q, want it to contain %q", i, got[i], w)
				}
			}
		})
	}
}

func openTemp(t *testing.T, path string, stmts ...string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return db
}

const assessmentDDL = `CREATE TABLE assessment (pin TEXT, year INTEGER, mailed REAL, certified REAL)`

// TestTake checks the inputs to the swap gate: a missing table is left out
// of Counts (so Check reports it missing), and the latest year is read.
func TestTake(t *testing.T) {
	db := openTemp(t, filepath.Join(t.TempDir(), "new.db"), assessmentDDL, `INSERT INTO assessment VALUES
		('A', 2026, 10, 9), ('B', 2026, 10, NULL), ('B', 2025, 10, NULL), ('E', 2026, 10, NULL)`)
	snap, err := Take(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Counts["assessment"] != 4 || snap.LatestYear != 2026 {
		t.Errorf("snapshot = %+v, want 4 assessment rows and 2026", snap)
	}
	if _, ok := snap.Counts["parcel"]; ok {
		t.Errorf("missing table should be absent from Counts")
	}
}

// TestFinalize checks the point of finalizing: the database becomes one
// self-contained file that can be renamed into place with nothing left in a
// WAL beside it.
func TestFinalize(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "f.db")
	db := openTemp(t, path, assessmentDDL, `INSERT INTO assessment VALUES ('A', 2026, 1, 1)`)
	if err := Finalize(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + "-wal"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("WAL file left beside the database (stat: %v)", err)
	}
	moved := filepath.Join(dir, "moved.db")
	if err := os.Rename(path, moved); err != nil {
		t.Fatal(err)
	}
	again := openTemp(t, moved)
	var n int
	if err := again.QueryRow(`SELECT COUNT(*) FROM assessment`).Scan(&n); err != nil || n != 1 {
		t.Errorf("after the move: %d rows (%v), want 1", n, err)
	}
}

// TestQuickCheckCorrupt checks that a damaged file fails the integrity check.
func TestQuickCheckCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.db")
	db := openTemp(t, path, assessmentDDL)
	for i := range 200 {
		if _, err := db.Exec(`INSERT INTO assessment VALUES (?, 2026, 1, 1)`, strings.Repeat("x", 100)+string(rune('a'+i%26))); err != nil {
			t.Fatal(err)
		}
	}
	if err := Finalize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	db.Close()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 4096; i < 2*4096; i++ { // scribble over the whole second page (the table's root)
		b[i] = 0xFF
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := QuickCheck(context.Background(), openTemp(t, path)); err == nil {
		t.Error("QuickCheck passed a corrupted database")
	}
}
