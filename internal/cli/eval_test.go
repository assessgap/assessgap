package cli

import (
	"testing"

	"github.com/assessgap/assessgap/internal/comps"
)

// TestBuckets: the backtest tables in docs/methodology.md, and the rates the
// reports quote from them, are keyed by these labels. A boundary that moved,
// or a groupOrder that no longer lines up with comps.Group's numbering, would
// put appeals in the wrong row without any other test failing.
func TestBuckets(t *testing.T) {
	groups := []struct {
		name string
		st   comps.Stats
		want string
	}{
		{"2 comps", comps.Stats{NumComps: 2, PctAboveMedian: 0.5, CompsLower: 2}, "<3 comps"},
		{"+1%, 6 of 8 lower", comps.Stats{NumComps: 8, PctAboveMedian: 0.01, CompsLower: 6}, ">= +1%, >= 3/4 lower"},
		{"+1%, 5 of 8 lower", comps.Stats{NumComps: 8, PctAboveMedian: 0.01, CompsLower: 5}, ">= +1%, < 3/4 lower"},
		{"+0.99%", comps.Stats{NumComps: 8, PctAboveMedian: 0.0099, CompsLower: 8}, "in line (±1%)"},
		{"-0.99%", comps.Stats{NumComps: 8, PctAboveMedian: -0.0099, CompsLower: 0}, "in line (±1%)"},
		{"-1%, 2 of 8 lower", comps.Stats{NumComps: 8, PctAboveMedian: -0.01, CompsLower: 2}, "< -1%, >= 1/4 lower"},
		{"-1%, 1 of 8 lower", comps.Stats{NumComps: 8, PctAboveMedian: -0.01, CompsLower: 1}, "< -1%, < 1/4 lower"},
	}
	for _, tt := range groups {
		if got := groupBucket(tt.st); got != tt.want {
			t.Errorf("groupBucket(%s) = %q, want %q", tt.name, got, tt.want)
		}
	}

	gaps := []struct {
		gap        float64
		n          int
		tier, band string
	}{
		{0.5, 2, "<3 comps", "<3 comps"},
		{0.30, 3, ">= +5%", ">= 30%"},
		{0.2999, 3, ">= +5%", "20..30%"},
		{0.05, 3, ">= +5%", "5..10%"},
		{0.0499, 3, "+1..+5%", "0..5%"},
		{0.01, 3, "+1..+5%", "0..5%"},
		{0.0099, 3, "in line (±1%)", "0..5%"},
		{0, 3, "in line (±1%)", "0..5%"},
		{-0.0001, 3, "in line (±1%)", "-5..0%"},
		{-0.01, 3, "< -1%", "-5..0%"},
		{-0.10, 3, "< -1%", "-10..-5%"},
		{-0.1001, 3, "< -1%", "< -10%"},
	}
	for _, tt := range gaps {
		if got := tierBucket(tt.gap, tt.n); got != tt.tier {
			t.Errorf("tierBucket(%v, %d) = %q, want %q", tt.gap, tt.n, got, tt.tier)
		}
		if got := gapBucket(tt.gap, tt.n); got != tt.band {
			t.Errorf("gapBucket(%v, %d) = %q, want %q", tt.gap, tt.n, got, tt.band)
		}
	}
}

func TestAUC(t *testing.T) {
	w := func(g float64) scored { return scored{g, true} }
	l := func(g float64) scored { return scored{g, false} }
	tests := []struct {
		name string
		in   []scored
		want float64
	}{
		// Every winner above every loser.
		{"perfect", []scored{l(0.1), l(0.2), w(0.3), w(0.4)}, 1},
		{"reversed", []scored{w(0.1), w(0.2), l(0.3), l(0.4)}, 0},
		// Winner/loser pairs: (0.1,0.2)=0, (0.1,0.3)=0, (0.3,0.2)=1, (0.3,0.3)=0.5 → 1.5/4.
		{"tie counts half", []scored{w(0.1), l(0.2), w(0.3), l(0.3)}, 0.375},
		{"no losers", []scored{w(0.1), w(0.2)}, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := auc(tt.in); got != tt.want {
				t.Errorf("auc = %v, want %v", got, tt.want)
			}
		})
	}
}
