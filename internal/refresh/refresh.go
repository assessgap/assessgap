// Package refresh validates a freshly built county database against the one
// currently in service, so the nightly job only swaps in a database that is at
// least as complete as the old one.
package refresh

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// Tables are the typed tables cmd/build produces. Each must be non-empty.
var Tables = []string{
	"parcel", "assessment", "res_char", "condo_char", "sale",
	"bor_appeal", "ccao_appeal", "nbhd_bor_stats",
}

// Snapshot is what we compare between the old and new database.
type Snapshot struct {
	Counts     map[string]int64 // table → rows; a missing table is absent from the map
	LatestYear int              // MAX(assessment.year)
}

// Take reads row counts for Tables and the latest assessment year.
// Missing tables are left out of Counts rather than treated as errors, so a
// new table can be added to build.sql without breaking the first comparison.
func Take(ctx context.Context, db *sql.DB) (Snapshot, error) {
	s := Snapshot{Counts: map[string]int64{}}
	for _, t := range Tables {
		var exists int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, t).Scan(&exists); err != nil {
			return s, err
		}
		if exists == 0 {
			continue
		}
		var n int64
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+t).Scan(&n); err != nil {
			return s, fmt.Errorf("count %s: %w", t, err)
		}
		s.Counts[t] = n
	}
	if _, ok := s.Counts["assessment"]; ok {
		var y sql.NullInt64
		if err := db.QueryRowContext(ctx, `SELECT MAX(year) FROM assessment`).Scan(&y); err != nil {
			return s, err
		}
		s.LatestYear = int(y.Int64)
	}
	return s, nil
}

// Check returns every reason the new snapshot must not replace the old one.
// old may be nil (first build): then only the non-empty rule applies.
// maxShrink is a fraction, e.g. 0.02 for 2%.
func Check(old *Snapshot, cur Snapshot, maxShrink float64) []string {
	var problems []string
	for _, t := range Tables {
		n, ok := cur.Counts[t]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s: table missing", t))
			continue
		case n == 0:
			problems = append(problems, fmt.Sprintf("%s: table is empty", t))
			continue
		}
		if old == nil {
			continue
		}
		if o, ok := old.Counts[t]; ok && o > 0 {
			if shrink := float64(o-n) / float64(o); shrink > maxShrink {
				problems = append(problems, fmt.Sprintf("%s: shrank %.1f%% (%d → %d), limit %.1f%%",
					t, 100*shrink, o, n, 100*maxShrink))
			}
		}
	}
	if old != nil && cur.LatestYear < old.LatestYear {
		problems = append(problems, fmt.Sprintf("assessment: latest year went backwards (%d → %d)",
			old.LatestYear, cur.LatestYear))
	}
	return problems
}

// Summary formats row counts old → new, one table per line.
func Summary(old *Snapshot, cur Snapshot) string {
	names := map[string]bool{}
	for t := range cur.Counts {
		names[t] = true
	}
	if old != nil {
		for t := range old.Counts {
			names[t] = true
		}
	}
	sorted := make([]string, 0, len(names))
	for t := range names {
		sorted = append(sorted, t)
	}
	sort.Strings(sorted)

	var b strings.Builder
	fmt.Fprintf(&b, "%-16s %12s %12s %10s\n", "table", "old", "new", "change")
	for _, t := range sorted {
		n, nok := cur.Counts[t]
		var o int64
		oldCol, change := "-", "new"
		if old != nil {
			if v, ok := old.Counts[t]; ok {
				o, oldCol = v, fmt.Sprint(v)
				change = fmt.Sprintf("%+d", n-o)
				if o > 0 {
					change += fmt.Sprintf(" (%+.2f%%)", 100*float64(n-o)/float64(o))
				}
			}
		}
		newCol := fmt.Sprint(n)
		if !nok {
			newCol, change = "missing", ""
		}
		fmt.Fprintf(&b, "%-16s %12s %12s %10s\n", t, oldCol, newCol, change)
	}
	if old != nil {
		fmt.Fprintf(&b, "latest assessment year: %d → %d\n", old.LatestYear, cur.LatestYear)
	} else {
		fmt.Fprintf(&b, "latest assessment year: %d\n", cur.LatestYear)
	}
	return b.String()
}

// StageChange counts PINs whose assessed value for year moved from mailed-only
// (certified NULL) in the old database to certified in the new one.
type StageChange struct {
	Year           int
	OldMailedOnly  int // PINs with certified NULL in old
	NewMailedOnly  int // PINs with certified NULL in new
	NowCertified   int // mailed-only in old, certified in new
	MissingFromNew int // mailed-only in old, absent from new
}

func (s StageChange) String() string {
	return fmt.Sprintf("%d mailed-only PINs: %d → %d; %d became certified; %d no longer present",
		s.Year, s.OldMailedOnly, s.NewMailedOnly, s.NowCertified, s.MissingFromNew)
}

// CertifiedSince compares mailed-only PINs for year between two databases.
// The databases are read separately (no ATTACH) so old can stay read-only.
func CertifiedSince(ctx context.Context, old, cur *sql.DB, year int) (StageChange, error) {
	sc := StageChange{Year: year}
	mailedOnly := map[string]bool{}
	rows, err := old.QueryContext(ctx,
		`SELECT pin FROM assessment WHERE year = ? AND certified IS NULL`, year)
	if err != nil {
		return sc, err
	}
	for rows.Next() {
		var pin string
		if err := rows.Scan(&pin); err != nil {
			rows.Close()
			return sc, err
		}
		mailedOnly[pin] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return sc, err
	}
	sc.OldMailedOnly = len(mailedOnly)

	rows, err = cur.QueryContext(ctx,
		`SELECT pin, certified IS NOT NULL FROM assessment WHERE year = ?`, year)
	if err != nil {
		return sc, err
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var pin string
		var certified bool
		if err := rows.Scan(&pin, &certified); err != nil {
			return sc, err
		}
		if !certified {
			sc.NewMailedOnly++
		}
		if mailedOnly[pin] {
			seen++
			if certified {
				sc.NowCertified++
			}
		}
	}
	sc.MissingFromNew = sc.OldMailedOnly - seen
	return sc, rows.Err()
}

// Finalize folds the WAL into the main file and switches it to rollback
// journaling, so the database is one self-contained file that can be renamed
// into place (no -wal/-shm to carry along).
func Finalize(ctx context.Context, db *sql.DB) error {
	var busy, logFrames, ckpt int
	if err := db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &logFrames, &ckpt); err != nil {
		return fmt.Errorf("checkpoint: %w", err)
	}
	if busy != 0 {
		return fmt.Errorf("checkpoint: database busy (another connection is open)")
	}
	var mode string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode = DELETE`).Scan(&mode); err != nil {
		return fmt.Errorf("journal_mode: %w", err)
	}
	if mode != "delete" {
		return fmt.Errorf("journal_mode: still %q", mode)
	}
	return nil
}

// QuickCheck runs PRAGMA quick_check and returns an error unless it says "ok".
func QuickCheck(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA quick_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var msgs []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(msgs) != 1 || msgs[0] != "ok" {
		return fmt.Errorf("quick_check: %s", strings.Join(msgs, "; "))
	}
	return nil
}
