package comps

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/assessgap/assessgap/internal/fixture"
)

func TestMain(m *testing.M) { os.Exit(fixture.Main(m)) }

// TestInvariantsEveryHome builds the report for every supported home in the
// fixture (about 2,100) and checks the rules every comparison must follow.
// The goldens pin exact numbers for a few homes; this catches a broken rule
// anywhere in real data, without a reviewer having to spot it in a diff.
func TestInvariantsEveryHome(t *testing.T) {
	db := fixture.Open(t)
	pins := allPINs(t, db)
	e := New(db)
	e.CachePools = true
	supported := 0
	for _, pin := range pins {
		r, err := e.Report(context.Background(), pin, 0)
		if err != nil {
			t.Fatalf("%s: %v", pin, err)
		}
		if r.Unsupported != "" {
			if r.Subject != nil || r.Stats != nil || r.Comps != nil || r.Verdict != "" {
				t.Errorf("%s: unsupported report has a comparison", pin)
			}
			continue
		}
		supported++
		checkInvariants(t, r)
	}
	// The fixture is frozen at 2,102 supported homes; this keeps the loop from
	// passing because it checked nothing.
	if supported != 2102 {
		t.Errorf("%d supported homes, want 2102", supported)
	}
}

func allPINs(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT DISTINCT pin FROM assessment ORDER BY pin`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var pins []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		pins = append(pins, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return pins
}

// checkInvariants asserts the rules in docs/methodology.md that hold for
// every supported report.
func checkInvariants(t *testing.T, r *Report) {
	t.Helper()
	s, st := r.Subject, r.Stats
	if len(r.Comps) > 30 || st.NumComps != len(r.Comps) {
		t.Errorf("%s: %d comps, NumComps %d", r.PIN, len(r.Comps), st.NumComps)
	}
	if len(r.Comps) != min(r.Search.Candidates, 30) {
		t.Errorf("%s: %d candidates for %d comps", r.PIN, r.Search.Candidates, len(r.Comps))
	}
	for i, c := range r.Comps {
		if c.PIN == s.PIN {
			t.Errorf("%s: subject listed as its own comp", r.PIN)
		}
		if c.NumCards != 1 || c.BldgSF <= 0 || c.AV <= 0 {
			t.Errorf("%s: comp %s has cards=%d sf=%v av=%v", r.PIN, c.PIN, c.NumCards, c.BldgSF, c.AV)
		}
		if classGroup[c.Class] != classGroup[s.Class] {
			t.Errorf("%s: comp %s class %s is outside the subject's group", r.PIN, c.PIN, c.Class)
		}
		if r.Search.SameClass && c.Class != s.Class {
			t.Errorf("%s: same-class search admitted class %s", r.PIN, c.Class)
		}
		if d := c.BldgSF - s.BldgSF; d > r.Search.SFTolerance*s.BldgSF || -d > r.Search.SFTolerance*s.BldgSF {
			t.Errorf("%s: comp %s sqft %v outside ±%v of %v", r.PIN, c.PIN, c.BldgSF, r.Search.SFTolerance, s.BldgSF)
		}
		if i > 0 {
			p := r.Comps[i-1]
			// Similarity first; then the nearer home (an unknown location, -1, last); then PIN.
			pm, cm := p.Miles, c.Miles
			if pm < 0 {
				pm = 1e9
			}
			if cm < 0 {
				cm = 1e9
			}
			if p.Distance > c.Distance || (p.Distance == c.Distance && (pm > cm || (pm == cm && p.PIN > c.PIN))) {
				t.Errorf("%s: comps not ordered by similarity, then miles, then PIN, at %d", r.PIN, i)
			}
		}
	}
	if len(r.Evidence) > 6 {
		t.Errorf("%s: %d lower-assessed homes listed, max 6", r.PIN, len(r.Evidence))
	}
	for _, c := range r.Evidence {
		if c.BldgAVPerSF >= s.BldgAVPerSF {
			t.Errorf("%s: listed %s as lower-assessed at %v ≥ %v", r.PIN, c.PIN, c.BldgAVPerSF, s.BldgAVPerSF)
		}
	}
	if st.PotentialAVReduction < 0 || st.SubjectPercentile < 0 || st.SubjectPercentile > 100 {
		t.Errorf("%s: reduction %v, percentile %v", r.PIN, st.PotentialAVReduction, st.SubjectPercentile)
	}
	switch r.Verdict {
	case Strong, Possible, Weak:
	default:
		t.Errorf("%s: verdict %q", r.PIN, r.Verdict)
	}
}

// home builds a test home built in 1950; sf is building sqft, av is total AV.
func home(pin, class string, sf, av float64) Home {
	return Home{PIN: pin, Class: class, BldgSF: sf, AV: av, BldgAV: av, HasSplit: true, BldgAVPerSF: av / sf, YearBuilt: 1950,
		NumCards: 1, FullBaths: 1, Rooms: 5, Beds: 2, LandSF: 3750,
		TypeResd: "1 Story", ExtWall: "Masonry", Basement: "Full"}
}

func TestSelectCompsWidening(t *testing.T) {
	subject := home("S", "203", 1200, 30000)
	tests := []struct {
		name      string
		pool      []Home
		wantLevel int
		wantN     int
		wantLast  string // PIN of the last comp kept, when it matters
	}{
		{
			name:      "enough same-class comps",
			pool:      repeat(10, func(i int) Home { return home(pin(i), "203", 1200+float64(i), 30000) }),
			wantLevel: 0, wantN: 10,
		},
		{
			name:      "falls back to class group",
			pool:      repeat(10, func(i int) Home { return home(pin(i), "202", 1100, 30000) }),
			wantLevel: 1, wantN: 10,
		},
		{
			name:      "widens sqft to ±35%",
			pool:      repeat(9, func(i int) Home { return home(pin(i), "203", 1550, 30000) }),
			wantLevel: 2, wantN: 9,
		},
		{
			name:      "never crosses class group",
			pool:      repeat(10, func(i int) Home { return home(pin(i), "205", 1200, 30000) }),
			wantLevel: 3, wantN: 0,
		},
		{
			// Distances are 1·i/1200 (plus the same 4 for an unknown
			// location), so the 30 most similar are c00–c29.
			name:      "keeps the 30 most similar",
			pool:      repeat(40, func(i int) Home { return home(pin(39-i), "203", 1200+float64(39-i), 30000) }),
			wantLevel: 0, wantN: 30, wantLast: "c29",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, s := selectComps(subject, tt.pool)
			if s.Level != tt.wantLevel || len(got) != tt.wantN {
				t.Fatalf("level %d, %d comps; want level %d, %d comps", s.Level, len(got), tt.wantLevel, tt.wantN)
			}
			if tt.wantLast != "" && got[len(got)-1].PIN != tt.wantLast {
				t.Errorf("last comp %s, want %s", got[len(got)-1].PIN, tt.wantLast)
			}
		})
	}
}

func TestVerdict(t *testing.T) {
	// Subject at $30/sf; comps at the given AV/sf values. The tier depends on
	// the share of comps strictly below $30.
	lower, higher := func(n int) []float64 { return fill(n, 25) }, func(n int) []float64 { return fill(n, 35) }
	tests := []struct {
		name  string
		comps []float64
		want  string
	}{
		{"strong: 8 of 10 lower", append(lower(8), higher(2)...), Strong},
		{"strong: exactly three quarters (9 of 12)", append(lower(9), higher(3)...), Strong},
		{"possible: just under three quarters (11 of 15)", append(lower(11), higher(4)...), Possible},
		{"possible: most lower but only 6 comps", append(lower(5), higher(1)...), Possible},
		{"possible: exactly half (5 of 10)", append(lower(5), higher(5)...), Possible},
		{"weak: just under half (4 of 9)", append(lower(4), higher(5)...), Weak},
		{"weak: assessed the same as every comp", fill(10, 30), Weak},
		{"weak: too few comps", fill(2, 20), Weak},
		{"weak: half lower but < 3 lower-assessed", append(lower(2), higher(2)...), Weak},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := home("S", "203", 1000, 30000)
			var cs []Comp
			for i, v := range tt.comps {
				cs = append(cs, Comp{Home: home(pin(i), "203", 1000, v*1000), Distance: float64(i)})
			}
			r := &Report{Subject: &s, Comps: cs, Search: &Search{}}
			st := computeStats(s, cs)
			r.Stats, r.Evidence = &st, evidenceComps(s, cs)
			got, why := decide(r)
			if got != tt.want {
				t.Errorf("verdict %s, want %s (gap %.4f, evidence %d)\n%s",
					got, tt.want, st.PctAboveMedian, len(r.Evidence), strings.Join(why, "\n"))
			}
		})
	}
}

func TestCharacteristicFlags(t *testing.T) {
	peers := repeat(12, func(i int) Home { return home(pin(i), "203", 1100, 30000) })
	tests := []struct {
		name   string
		peers  []Home
		mutate func(*Home)
		want   []string
	}{
		{"typical home", peers, func(*Home) {}, nil},
		{"sqft far above peers", peers, func(h *Home) { h.BldgSF = 2400 }, []string{"bldg_sf"}},
		{"sqft far below peers", peers, func(h *Home) { h.BldgSF = 600 }, []string{"bldg_sf"}},
		{"sqft outlier but under 10 peers", peers[:9], func(h *Home) { h.BldgSF = 2400 }, nil},
		{"more beds than rooms", peers, func(h *Home) { h.Beds = 6 }, []string{"beds"}},
		{"no full baths", peers, func(h *Home) { h.FullBaths = 0 }, []string{"fbath"}},
		{"missing year built", peers, func(h *Home) { h.YearBuilt = 0 }, []string{"yrblt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := home("S", "203", 1100, 30000)
			tt.mutate(&s)
			var got []string
			for _, f := range characteristicFlags(s, tt.peers) {
				got = append(got, f.Field)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("flags %v, want %v", got, tt.want)
			}
		})
	}
}

func repeat(n int, f func(int) Home) []Home {
	out := make([]Home, n)
	for i := range out {
		out[i] = f(i)
	}
	return out
}

func fill(n int, v float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func pin(i int) string { return fmt.Sprintf("c%02d", i) }
