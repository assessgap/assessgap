package comps

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/assessgap/assessgap/internal/store"
)

// The sample DB is PIN prefix 1328 (Jefferson, nbhd 71260). Build it with:
//
//	go run ./cmd/ingest -data data-sample -pin-prefix 1328 all && go run ./cmd/build -db data-sample/cook.db
const sampleDB = "../../data-sample/cook.db"

func openSample(t *testing.T) *sql.DB {
	t.Helper()
	if _, err := os.Stat(sampleDB); err != nil {
		t.Skipf("sample DB missing (%v)", err)
	}
	db, err := store.Open(sampleDB)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestReportSample(t *testing.T) {
	e := New(openSample(t))
	ctx := context.Background()

	tests := []struct {
		name        string
		pin         string
		year        int
		wantErr     error
		unsupported string // substring; "" means supported
		wantStage   string
	}{
		{name: "one-story bungalow, latest year", pin: "13281000270000", wantStage: "mailed"},
		{name: "one-story bungalow, certified year", pin: "13281000270000", year: 2025, wantStage: "certified"},
		{name: "two-story older home", pin: "13281020260000", year: 2025, wantStage: "certified"},
		{name: "condo", pin: "13281010421001", year: 2025, unsupported: "Condominium"},
		{name: "commercial", pin: "13281000010000", year: 2025, unsupported: "not residential"},
		{name: "two-flat", pin: "13281000280000", year: 2025, unsupported: "Multi-family"},
		{name: "multi-card home", pin: "13281130150000", year: 2025, unsupported: "more than one building"},
		{name: "unknown PIN", pin: "99999999999999", wantErr: ErrNotFound},
		{name: "known PIN, missing year", pin: "13281000270000", year: 1999, wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := e.Report(ctx, tt.pin, tt.year)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if r.Disclaimer == "" {
				t.Error("missing disclaimer")
			}
			if tt.unsupported != "" {
				if !strings.Contains(r.Unsupported, tt.unsupported) {
					t.Fatalf("Unsupported = %q, want it to mention %q", r.Unsupported, tt.unsupported)
				}
				if r.Verdict != "" || r.Comps != nil {
					t.Error("unsupported report should have no verdict or comps")
				}
				return
			}
			if r.Unsupported != "" {
				t.Fatalf("unexpected Unsupported: %s", r.Unsupported)
			}
			if r.AVStage != tt.wantStage {
				t.Errorf("AVStage = %q, want %q", r.AVStage, tt.wantStage)
			}
			checkInvariants(t, r)
		})
	}
}

// checkInvariants asserts properties every supported report must have.
func checkInvariants(t *testing.T, r *Report) {
	t.Helper()
	s := r.Subject
	if len(r.Comps) < minComps || len(r.Comps) > maxComps {
		t.Errorf("got %d comps, want %d–%d", len(r.Comps), minComps, maxComps)
	}
	if !sort.SliceIsSorted(r.Comps, func(i, j int) bool { return r.Comps[i].Distance < r.Comps[j].Distance }) {
		t.Error("comps not sorted by distance")
	}
	for _, c := range r.Comps {
		if c.PIN == s.PIN {
			t.Error("subject listed as its own comp")
		}
		if c.NumCards != 1 || c.BldgSF <= 0 {
			t.Errorf("comp %s: cards=%d sf=%v", c.PIN, c.NumCards, c.BldgSF)
		}
		if classGroup[c.Class] != classGroup[s.Class] {
			t.Errorf("comp %s class %s outside subject's group", c.PIN, c.Class)
		}
		if d := c.BldgSF/s.BldgSF - 1; abs64(d) > r.Search.SFTolerance+1e-9 {
			t.Errorf("comp %s sqft off by %.0f%%", c.PIN, 100*d)
		}
	}
	if len(r.Evidence) > maxEvidence {
		t.Errorf("%d evidence comps, max %d", len(r.Evidence), maxEvidence)
	}
	for _, c := range r.Evidence {
		if c.AVPerSF >= s.AVPerSF {
			t.Errorf("evidence comp %s not below subject AV/sf", c.PIN)
		}
	}
	st := r.Stats
	if st.PotentialAVReduction < 0 {
		t.Error("negative potential reduction")
	}
	if want := round(st.MedianAVPerSF*s.BldgSF, 0); abs64(st.UniformityAV-want) > s.BldgSF*0.01 {
		t.Errorf("UniformityAV = %v, want ≈ %v", st.UniformityAV, want)
	}
	if st.SubjectPercentile < 0 || st.SubjectPercentile > 100 {
		t.Errorf("percentile %v", st.SubjectPercentile)
	}
	switch r.Verdict {
	case Strong, Possible, Weak:
	default:
		t.Errorf("verdict %q", r.Verdict)
	}
	if len(r.Reasons) == 0 {
		t.Error("no reasons")
	}
	for _, why := range append(r.Reasons, flagMessages(r.Flags)...) {
		for _, banned := range []string{"market value", "appraisal", "worth", "appraised"} {
			if strings.Contains(strings.ToLower(why), banned) {
				t.Errorf("report text mentions %q: %s", banned, why)
			}
		}
	}
	if len(r.NbhdAppeals) == 0 {
		t.Error("no neighborhood appeal stats")
	}
}

func flagMessages(fs []Flag) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Message)
	}
	return out
}

// home builds a test home; sf is building sqft, av is total AV.
func home(pin, class string, sf, av float64, yr int) Home {
	return Home{PIN: pin, Class: class, BldgSF: sf, AV: av, AVPerSF: av / sf, YearBuilt: yr,
		NumCards: 1, FullBaths: 1, Rooms: 5, Beds: 2, LandSF: 3750,
		TypeResd: "1 Story", ExtWall: "Masonry", Basement: "Full"}
}

func TestSelectCompsWidening(t *testing.T) {
	subject := home("S", "203", 1200, 30000, 1950)
	tests := []struct {
		name      string
		pool      []Home
		wantLevel int
		wantN     int
	}{
		{
			name:      "enough same-class comps",
			pool:      repeat(10, func(i int) Home { return home(pin(i), "203", 1200+float64(i), 30000, 1950) }),
			wantLevel: 0, wantN: 10,
		},
		{
			name:      "falls back to class group",
			pool:      repeat(10, func(i int) Home { return home(pin(i), "202", 1100, 30000, 1950) }),
			wantLevel: 1, wantN: 10,
		},
		{
			name:      "widens sqft to ±35%",
			pool:      repeat(9, func(i int) Home { return home(pin(i), "203", 1550, 30000, 1950) }),
			wantLevel: 2, wantN: 9,
		},
		{
			name:      "never crosses class group",
			pool:      repeat(10, func(i int) Home { return home(pin(i), "205", 1200, 30000, 1950) }),
			wantLevel: 3, wantN: 0,
		},
		{
			name: "excludes multi-card, zero sqft, and subject",
			pool: []Home{
				subject,
				func() Home { h := home("M", "203", 1200, 30000, 1950); h.NumCards = 2; return h }(),
				home("Z", "203", 0, 30000, 1950),
			},
			wantLevel: 3, wantN: 0,
		},
		{
			name:      "keeps top 20",
			pool:      repeat(30, func(i int) Home { return home(pin(i), "203", 1200+float64(i), 30000, 1950) }),
			wantLevel: 0, wantN: 20,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, s := selectComps(subject, tt.pool)
			if s.Level != tt.wantLevel || len(got) != tt.wantN {
				t.Errorf("level %d, %d comps; want level %d, %d comps", s.Level, len(got), tt.wantLevel, tt.wantN)
			}
		})
	}
}

func TestVerdict(t *testing.T) {
	// Subject at $30/sf; comps at the given AV/sf values.
	tests := []struct {
		name  string
		comps []float64
		want  string
	}{
		{"strong: 20% above, 10 comps", fill(10, 25), Strong},
		{"strong: 7% above", fill(10, 28), Strong},
		{"possible: 3% above", fill(10, 29.1), Possible},
		{"possible: big gap but only 6 comps", fill(6, 25), Possible},
		{"possible: 2% above", fill(10, 29.4), Possible},
		{"weak: in line (<1% above)", fill(10, 29.8), Weak},
		{"weak: below median", fill(10, 33), Weak},
		{"weak: too few comps", fill(2, 20), Weak},
		{"weak: gap but < 3 evidence", []float64{20, 20, 31, 31}, Weak},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := home("S", "203", 1000, 30000, 1950)
			var cs []Comp
			for i, v := range tt.comps {
				cs = append(cs, Comp{Home: home(pin(i), "203", 1000, v*1000, 1950), Distance: float64(i)})
			}
			r := &Report{Subject: &s, Comps: cs, Search: &Search{}}
			st := computeStats(s, cs)
			r.Stats, r.Evidence = &st, evidenceComps(s, cs)
			got, why := decide(r)
			if got != tt.want {
				t.Errorf("verdict %s, want %s (gap %.3f, evidence %d)\n%s",
					got, tt.want, st.PctAboveMedian, len(r.Evidence), strings.Join(why, "\n"))
			}
		})
	}
}

func TestCharacteristicFlags(t *testing.T) {
	peers := repeat(12, func(i int) Home { return home(pin(i), "203", 1100, 30000, 1950) })
	tests := []struct {
		name   string
		mutate func(*Home)
		want   []string
	}{
		{"typical home", func(*Home) {}, nil},
		{"sqft far above peers", func(h *Home) { h.BldgSF = 2400 }, []string{"bldg_sf"}},
		{"sqft far below peers", func(h *Home) { h.BldgSF = 600 }, []string{"bldg_sf"}},
		{"more beds than rooms", func(h *Home) { h.Beds = 6 }, []string{"beds"}},
		{"no full baths", func(h *Home) { h.FullBaths = 0 }, []string{"fbath"}},
		{"missing year built", func(h *Home) { h.YearBuilt = 0 }, []string{"yrblt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := home("S", "203", 1100, 30000, 1950)
			tt.mutate(&s)
			var got []string
			for _, f := range characteristicFlags(s, peers) {
				got = append(got, f.Field)
				if !strings.Contains(f.Message, "Please verify") {
					t.Errorf("flag not phrased as a request: %s", f.Message)
				}
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
