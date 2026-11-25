package main

import (
	"context"
	"database/sql"
	"encoding/gob"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/store"
)

// Tuning years and the held-out year. The holdout's appeals and pools are
// written to a separate file that only the holdout command opens.
var (
	tuneYears    = []int{2023, 2024}
	holdoutYears = []int{2025}
)

// Appeal is one PIN-year decided by the Board of Review, with everything the
// tuning needs that was on record before the Board's decision. Won and Cut
// are the outcome: they are only ever used as labels, never as features.
type Appeal struct {
	PIN  string
	Year int
	Nbhd string // the year's Assessor neighborhood (as a report uses)
	Town string // township code: the neighborhood's first two digits
	Pro  bool   // filed by the homeowner
	Won  bool   // label: any Board reduction
	Cut  float64

	Subj int // index of the subject in its pool, -1 if a report would be unsupported

	// Context, all known before the Board decides tax year Year.
	Mailed, Certified float64 // the Assessor's values for Year: the Board starts from Certified
	PriorFinal        float64 // final (Board) total for Year-1, 0 if none
	PriorAppeals      int     // Board appeals on this PIN in Year-2..Year-1
	PriorWins         int     // of those, reductions
	NbhdN, NbhdWins   int     // pro se appeals in Year-2..Year-1 by homes in this Year's neighborhood
	Triad             string  // 1 City, 2 North, 3 South
}

// Dataset is a cached set of appeals and their neighborhoods' pools.
type Dataset struct {
	Years   []int
	Appeals []Appeal
	Pools   map[string][]comps.Home // key poolKey(nbhd, year)
}

func poolKey(nbhd string, year int) string { return fmt.Sprintf("%s/%d", nbhd, year) }

func (d *Dataset) pool(a Appeal) []comps.Home { return d.Pools[poolKey(a.Nbhd, a.Year)] }

// appealsSQL mirrors the backtest's (internal/cli/eval.go backtestSQL): one row
// per PIN and tax year, any decrease is a win, pro se if any filing was.
// The joins add the year's Assessor values and the prior year's final value.
var appealsSQL = `
SELECT b.pin, b.tax_year,
       MAX(b.result = 'Decrease'),
       MAX(b.pro_se),
       MAX(CASE WHEN b.result = 'Decrease' AND b.av_before > 0
                THEN (b.av_before - b.av_after) / b.av_before END),
       COALESCE(a.nbhd, ''), COALESCE(a.mailed, 0), COALESCE(a.certified, 0),
       COALESCE(pa.board, pa.certified, 0)
FROM bor_appeal b
LEFT JOIN assessment a  ON a.pin = b.pin AND a.year = b.tax_year
LEFT JOIN assessment pa ON pa.pin = b.pin AND pa.year = b.tax_year - 1
WHERE b.tax_year = ?
  AND substr(b.class, 1, 3) IN ('` + strings.Join(comps.ScopeClasses(), "','") + `')
  AND b.result IN ('Decrease', 'No Change')
GROUP BY b.pin, b.tax_year
ORDER BY b.pin`

// priorSQL lists every Board appeal on any class in two earlier tax years.
const priorSQL = `
SELECT pin, tax_year, MAX(result = 'Decrease')
FROM bor_appeal WHERE tax_year IN (?, ?)
GROUP BY pin, tax_year`

// nbhdHistorySQL counts pro se in-scope decided appeals in two earlier tax
// years, grouped by the neighborhood each home is in for the target year.
var nbhdHistorySQL = `
SELECT a.nbhd, COUNT(*), SUM(h.won)
FROM (SELECT pin, MAX(result = 'Decrease') AS won
      FROM bor_appeal
      WHERE tax_year IN (?, ?) AND pro_se = 1
        AND substr(class, 1, 3) IN ('` + strings.Join(comps.ScopeClasses(), "','") + `')
        AND result IN ('Decrease', 'No Change')
      GROUP BY pin, tax_year) h
JOIN assessment a ON a.pin = h.pin AND a.year = ?
GROUP BY a.nbhd`

const triadSQL = `SELECT DISTINCT township_code, triad_code FROM raw_parcel_universe`

func prep(ctx context.Context, dbPath, dir string) error {
	db, err := store.OpenReadOnly(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	e := comps.New(db)
	e.CachePools = false

	triads := map[string]string{}
	rows, err := db.QueryContext(ctx, triadSQL)
	if err != nil {
		return err
	}
	for rows.Next() {
		var t, c sql.NullString
		if err := rows.Scan(&t, &c); err != nil {
			return err
		}
		triads[t.String] = c.String
	}
	rows.Close()

	for _, set := range []struct {
		file  string
		years []int
	}{{"tune.gob", tuneYears}, {"holdout.gob", holdoutYears}} {
		d := &Dataset{Years: set.years, Pools: map[string][]comps.Home{}}
		for _, y := range set.years {
			as, err := loadYear(ctx, db, y)
			if err != nil {
				return fmt.Errorf("appeals %d: %w", y, err)
			}
			for i := range as {
				as[i].Triad = triads[as[i].Town]
			}
			d.Appeals = append(d.Appeals, as...)
		}
		// Pools: one query per neighborhood-year, through the engine's own loader.
		for i := range d.Appeals {
			a := &d.Appeals[i]
			a.Subj = -1
			if a.Nbhd == "" {
				continue
			}
			k := poolKey(a.Nbhd, a.Year)
			p, ok := d.Pools[k]
			if !ok {
				if p, err = e.Pool(ctx, a.Nbhd, a.Year); err != nil {
					return err
				}
				d.Pools[k] = p
			}
			a.Subj = subjectIndex(p, a.PIN)
		}
		if err := save(filepath.Join(dir, set.file), d); err != nil {
			return err
		}
		log.Printf("%s: %d appeals, %d pools", set.file, len(d.Appeals), len(d.Pools))
	}
	return nil
}

// subjectIndex finds a supported subject in its pool. The pool holds every
// in-scope home with characteristics, so a subject missing from it is out of
// scope or has no characteristics; the other checks are the report's own
// (comps.unsupported): one building, square footage, an AV and a split.
func subjectIndex(pool []comps.Home, pin string) int {
	for i, h := range pool {
		if h.PIN == pin {
			if h.NumCards == 1 && h.BldgSF > 0 && h.AV > 0 && h.HasSplit {
				return i
			}
			return -1
		}
	}
	return -1
}

func loadYear(ctx context.Context, db *sql.DB, y int) ([]Appeal, error) {
	rows, err := db.QueryContext(ctx, appealsSQL, y)
	if err != nil {
		return nil, err
	}
	var as []Appeal
	for rows.Next() {
		var a Appeal
		var cut sql.NullFloat64
		if err := rows.Scan(&a.PIN, &a.Year, &a.Won, &a.Pro, &cut, &a.Nbhd, &a.Mailed, &a.Certified, &a.PriorFinal); err != nil {
			rows.Close()
			return nil, err
		}
		a.Cut = math.NaN()
		if cut.Valid {
			a.Cut = cut.Float64
		}
		if len(a.Nbhd) >= 2 {
			a.Town = a.Nbhd[:2]
		}
		as = append(as, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Earlier appeals on the same PIN (decided before this year's).
	type py struct {
		n, wins int
	}
	prior := map[string]py{}
	rows, err = db.QueryContext(ctx, priorSQL, y-2, y-1)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var pin string
		var yr, won int
		if err := rows.Scan(&pin, &yr, &won); err != nil {
			rows.Close()
			return nil, err
		}
		p := prior[pin]
		p.n++
		p.wins += won
		prior[pin] = p
	}
	rows.Close()

	hist := map[string][2]int{}
	rows, err = db.QueryContext(ctx, nbhdHistorySQL, y-2, y-1, y)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var nb sql.NullString
		var n, w int
		if err := rows.Scan(&nb, &n, &w); err != nil {
			rows.Close()
			return nil, err
		}
		hist[nb.String] = [2]int{n, w}
	}
	rows.Close()

	for i := range as {
		a := &as[i]
		p := prior[a.PIN]
		a.PriorAppeals, a.PriorWins = p.n, p.wins
		h := hist[a.Nbhd]
		a.NbhdN, a.NbhdWins = h[0], h[1]
	}
	sort.Slice(as, func(i, j int) bool { return as[i].PIN < as[j].PIN })
	return as, nil
}

func save(path string, d *Dataset) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := gob.NewEncoder(f).Encode(d); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func load(path string) (*Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var d Dataset
	if err := gob.NewDecoder(f).Decode(&d); err != nil {
		return nil, err
	}
	return &d, nil
}

// loadTuning opens only the tuning file and refuses one that holds a
// held-out year, so no search command can see the holdout.
func loadTuning(dir string) (*Dataset, error) {
	d, err := load(filepath.Join(dir, "tune.gob"))
	if err != nil {
		return nil, err
	}
	for _, a := range d.Appeals {
		for _, h := range holdoutYears {
			if a.Year == h {
				return nil, fmt.Errorf("tune.gob holds a held-out year (%d)", h)
			}
		}
	}
	return d, nil
}
