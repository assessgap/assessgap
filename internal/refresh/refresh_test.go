package refresh

import (
	"context"
	"database/sql"
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

func TestSummary(t *testing.T) {
	old := &Snapshot{Counts: map[string]int64{"parcel": 100, "sale": 50}, LatestYear: 2025}
	cur := Snapshot{Counts: map[string]int64{"parcel": 110, "tax_rate": 7}, LatestYear: 2026}
	s := Summary(old, cur)
	for _, want := range []string{"parcel", "+10 (+10.00%)", "tax_rate", "new", "sale", "missing", "2025 → 2026"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary missing %q:\n%s", want, s)
		}
	}
}

func openTemp(t *testing.T, name string, stmts ...string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), name)+"?_pragma=journal_mode(WAL)")
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

func TestCertifiedSinceAndTake(t *testing.T) {
	ctx := context.Background()
	old := openTemp(t, "old.db", assessmentDDL, `INSERT INTO assessment VALUES
		('A', 2026, 10, NULL), ('B', 2026, 10, NULL), ('C', 2026, 10, NULL), ('D', 2026, 10, 9),
		('A', 2025, 10, NULL)`)
	cur := openTemp(t, "new.db", assessmentDDL, `INSERT INTO assessment VALUES
		('A', 2026, 10, 9), ('B', 2026, 10, NULL), ('D', 2026, 10, 9), ('E', 2026, 10, NULL)`)

	got, err := CertifiedSince(ctx, old, cur, 2026)
	if err != nil {
		t.Fatal(err)
	}
	want := StageChange{Year: 2026, OldMailedOnly: 3, NewMailedOnly: 2, NowCertified: 1, MissingFromNew: 1}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}

	snap, err := Take(ctx, cur)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Counts["assessment"] != 4 || snap.LatestYear != 2026 {
		t.Errorf("snapshot = %+v", snap)
	}
	if _, ok := snap.Counts["parcel"]; ok {
		t.Errorf("missing table should be absent from Counts")
	}
}

func TestFinalize(t *testing.T) {
	ctx := context.Background()
	db := openTemp(t, "f.db", assessmentDDL, `INSERT INTO assessment VALUES ('A', 2026, 1, 1)`)
	if err := Finalize(ctx, db); err != nil {
		t.Fatal(err)
	}
	var mode string
	db.QueryRow(`PRAGMA journal_mode`).Scan(&mode)
	if mode != "delete" {
		t.Errorf("journal_mode = %q, want delete", mode)
	}
	if err := QuickCheck(ctx, db); err != nil {
		t.Error(err)
	}
}
