package comps

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/assessgap/assessgap/internal/ingest"
	"github.com/assessgap/assessgap/internal/store"
)

// The synthetic builder makes a tiny in-memory county database of hand-written
// homes, for edge cases the real-data fixture doesn't reach cleanly. Expected
// numbers in these tests are worked out by hand, never by calling the engine.

const synthYear = 2026

type synth struct {
	t  *testing.T
	db *sql.DB
}

// newSynth returns an empty database with build.sql's typed tables.
func newSynth(t *testing.T) *synth {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, d := range ingest.Datasets {
		cols := ingest.Columns[d.Name]
		if _, err := db.Exec(fmt.Sprintf(`CREATE TABLE raw_%s ("%s")`, d.Name, strings.Join(cols, `", "`))); err != nil {
			t.Fatal(err)
		}
	}
	if err := ingest.Build(context.Background(), db, t.Logf); err != nil {
		t.Fatal(err)
	}
	return &synth{t: t, db: db}
}

// sh is one hand-written home. Zero fields mean "not on record" where the
// engine treats them that way (year built, land).
type sh struct {
	pin, class string
	sf, av     float64
	year       int
	cards      int  // 0 means 1
	noChars    bool // no res_char row at all
	certified  bool // AV is the certified value (else mailed)
	fbath      int  // 0 means 1, unless zeroBaths
	zeroBaths  bool
}

func (s *synth) add(h sh) {
	s.t.Helper()
	exec := func(q string, args ...any) {
		s.t.Helper()
		if _, err := s.db.Exec(q, args...); err != nil {
			s.t.Fatal(err)
		}
	}
	var mailed, cert any = h.av, nil
	if h.certified {
		cert = h.av
	}
	exec(`INSERT INTO assessment (pin, year, class, nbhd, mailed, certified) VALUES (?, ?, ?, '99001', ?, ?)`,
		h.pin, synthYear, h.class, mailed, cert)
	exec(`INSERT INTO parcel (pin, class, township_name, nbhd, address, city) VALUES (?, ?, 'Testville', '99001', ?, 'TESTVILLE')`,
		h.pin, h.class, h.pin+" TEST ST")
	if h.noChars {
		return
	}
	cards, fb := max(h.cards, 1), h.fbath
	if fb == 0 && !h.zeroBaths {
		fb = 1
	}
	exec(`INSERT INTO res_char (pin, year, class, bldg_sf, land_sf, yrblt, beds, fbath, hbath, rooms,
		type_resd, ext_wall, bsmt, num_cards) VALUES (?, ?, ?, ?, 3000, ?, 3, ?, 0, 6, '1 Story', 'Frame', 'Full', ?)`,
		h.pin, synthYear, h.class, h.sf, h.year, fb, cards)
}

func (s *synth) report(pin string) *Report {
	s.t.Helper()
	r, err := New(s.db).Report(context.Background(), pin, 0)
	if err != nil {
		s.t.Fatal(err)
	}
	return r
}

func TestSyntheticTwoComps(t *testing.T) {
	s := newSynth(t)
	s.add(sh{pin: "S", class: "203", sf: 1000, av: 30000, year: 1950}) // $30.00/sq ft
	s.add(sh{pin: "A", class: "203", sf: 1000, av: 20000, year: 1950}) // $20.00
	s.add(sh{pin: "B", class: "203", sf: 1000, av: 25000, year: 1950}) // $25.00
	r := s.report("S")

	// Two comps never reach 8, so the search widens to the last step.
	if r.Search.Level != 3 || r.Stats.NumComps != 2 {
		t.Fatalf("level %d, %d comps; want level 3, 2 comps", r.Search.Level, r.Stats.NumComps)
	}
	want := Stats{NumComps: 2, SubjectAVPerSF: 30, MedianAVPerSF: 22.5, PctAboveMedian: 0.3333,
		SubjectPercentile: 100, UniformityAV: 22500, PotentialAVReduction: 7500}
	if *r.Stats != want {
		t.Errorf("stats %+v\nwant  %+v", *r.Stats, want)
	}
	// Fewer than 3 comps: lowest tier whatever the gap, and only the
	// too-few explanation (no median or tax comparison).
	if r.Verdict != Weak || len(r.Reasons) != 1 {
		t.Errorf("verdict %s, reasons %q", r.Verdict, r.Reasons)
	}
}

func TestSyntheticTieAtMedian(t *testing.T) {
	s := newSynth(t)
	s.add(sh{pin: "S", class: "203", sf: 1000, av: 25000, year: 1950})
	for i, av := range []float64{20000, 25000, 25000, 30000} {
		s.add(sh{pin: fmt.Sprintf("C%d", i), class: "203", sf: 1000, av: av, year: 1950})
	}
	r := s.report("S")
	// Median of 20, 25, 25, 30 is (25+25)/2 = 25: exactly the subject.
	// Percentile = (1 below + 0.5 × 2 equal) / 4 = 50.
	want := Stats{NumComps: 4, SubjectAVPerSF: 25, MedianAVPerSF: 25, PctAboveMedian: 0,
		SubjectPercentile: 50, UniformityAV: 25000, PotentialAVReduction: 0}
	if *r.Stats != want {
		t.Errorf("stats %+v\nwant  %+v", *r.Stats, want)
	}
	// Only the $20 home is strictly lower; equal homes are not evidence.
	if len(r.Evidence) != 1 || r.Evidence[0].PIN != "C0" {
		t.Errorf("evidence %v, want [C0]", pins(r.Evidence))
	}
	if r.Verdict != Weak {
		t.Errorf("verdict %s, want weak", r.Verdict)
	}
}

func TestSyntheticEvenMedianBetween(t *testing.T) {
	s := newSynth(t)
	s.add(sh{pin: "S", class: "203", sf: 1200, av: 36000, year: 1950}) // $30.00
	// $29, $20, $22, $23: out of order, and chosen so the mean ($23.50), the
	// midrange ($24.50) and an unsorted middle pair ($21) all differ from the median.
	for i, av := range []float64{34800, 24000, 26400, 27600} {
		s.add(sh{pin: fmt.Sprintf("C%d", i), class: "203", sf: 1200, av: av, year: 1950})
	}
	r := s.report("S")
	// Median = ($22 + $23) / 2 = $22.50; gap 30/22.5 − 1 = 33.33%;
	// AV at the median rate = 22.5 × 1,200 = 27,000; difference 9,000.
	if r.Stats.MedianAVPerSF != 22.5 || r.Stats.PctAboveMedian != 0.3333 ||
		r.Stats.UniformityAV != 27000 || r.Stats.PotentialAVReduction != 9000 {
		t.Errorf("stats %+v", *r.Stats)
	}
	if len(r.Evidence) != 4 {
		t.Errorf("%d lower-assessed homes, want all 4", len(r.Evidence))
	}
}

func TestSyntheticNoComps(t *testing.T) {
	s := newSynth(t)
	s.add(sh{pin: "S", class: "203", sf: 1000, av: 30000, year: 1950})
	s.add(sh{pin: "T", class: "295", sf: 1000, av: 10000, year: 1950}) // townhome: another class group
	r := s.report("S")
	// No median to divide by: every comparison figure stays zero (not NaN or
	// Inf, and no reduction equal to the whole AV).
	if *r.Stats != (Stats{SubjectAVPerSF: 30}) || len(r.Comps) != 0 {
		t.Errorf("stats %+v, %d comps; want only the subject's $30", *r.Stats, len(r.Comps))
	}
	if r.Verdict != Weak || len(r.Reasons) != 1 {
		t.Errorf("verdict %s, reasons %q", r.Verdict, r.Reasons)
	}
}

func TestSyntheticUnsupported(t *testing.T) {
	s := newSynth(t)
	s.add(sh{pin: "NOCHARS", class: "203", av: 30000, noChars: true})
	s.add(sh{pin: "ZEROSF", class: "203", sf: 0, av: 30000, year: 1950})
	s.add(sh{pin: "NOAV", class: "203", sf: 1000, av: 0, year: 1950})
	// The keyword tells the branches apart; the full wording is not the point.
	tests := []struct{ pin, keyword string }{
		{"NOCHARS", "characteristics"},
		{"ZEROSF", "square footage"},
		{"NOAV", "assessed value"},
	}
	for _, tt := range tests {
		t.Run(tt.pin, func(t *testing.T) {
			r := s.report(tt.pin)
			if !strings.Contains(r.Unsupported, tt.keyword) || r.Stats != nil || r.Subject != nil {
				t.Errorf("unsupported %q (stats %v); want the %s reason and no comparison", r.Unsupported, r.Stats, tt.keyword)
			}
		})
	}
}

func TestSyntheticPoolExclusions(t *testing.T) {
	s := newSynth(t)
	s.add(sh{pin: "S", class: "203", sf: 1000, av: 30000, year: 1950})
	s.add(sh{pin: "OK", class: "203", sf: 1000, av: 25000, year: 1950})
	s.add(sh{pin: "ZEROSF", class: "203", sf: 0, av: 25000, year: 1950})
	s.add(sh{pin: "CARDS", class: "203", sf: 1000, av: 25000, year: 1950, cards: 2})
	s.add(sh{pin: "NOCHARS", class: "203", av: 25000, noChars: true})
	s.add(sh{pin: "NOAV", class: "203", sf: 1000, av: 0, year: 1950})
	r := s.report("S")
	if got := pins(r.Comps); got != "OK" {
		t.Errorf("comps %s, want only OK", got)
	}
}

func TestSyntheticUnknownYearBuilt(t *testing.T) {
	s := newSynth(t)
	s.add(sh{pin: "S", class: "203", sf: 1000, av: 30000, year: 0})
	s.add(sh{pin: "OLD", class: "203", sf: 1000, av: 25000, year: 1880}) // 140 years apart from NEW
	s.add(sh{pin: "NEW", class: "203", sf: 1000, av: 25000, year: 2020})
	r := s.report("S")
	// With the subject's year unknown, the age filter is skipped and age adds no distance.
	if got := pins(r.Comps); got != "NEW,OLD" {
		t.Errorf("comps %s, want NEW,OLD (both at distance 0, so by PIN)", got)
	}
	for _, c := range r.Comps {
		if c.Distance != 0 {
			t.Errorf("%s distance %v, want 0", c.PIN, c.Distance)
		}
	}
}

func pins(cs []Comp) string {
	var out []string
	for _, c := range cs {
		out = append(out, c.PIN)
	}
	return strings.Join(out, ",")
}
