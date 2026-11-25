package web

import (
	"testing"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/web/views"
)

// TestChartIdenticalValues covers the degenerate chart where every home has
// the same AV per sq ft: the range is padded to $24–$26 instead of dividing
// by zero, and 20 dots at one x stack into at most chartRows rows.
func TestChartIdenticalValues(t *testing.T) {
	s := comps.Home{PIN: "S", AVPerSF: 25}
	var cs []comps.Comp
	for i := range 20 {
		cs = append(cs, comps.Comp{Home: comps.Home{PIN: string(rune('a' + i)), AVPerSF: 25}})
	}
	tests := []struct {
		name  string
		l     views.ChartLayout
		x     float64 // Left + (25−24)/(26−24) × (Right − Left)
		ticks int     // $0.50 steps wide, $1 narrow
	}{
		{"wide", wideChart, 500, 5},
		{"narrow", narrowChart, 180, 3},
	}
	for _, tt := range tests {
		ch := newChart(s, cs, nil, 25, tt.l)
		if ch.SubjectX != tt.x || ch.MedianX != tt.x || len(ch.Dots) != 20 || len(ch.Ticks) != tt.ticks {
			t.Errorf("%s: subject x %v, median x %v, %d dots, %d ticks; want x %v, 20 dots, %d ticks",
				tt.name, ch.SubjectX, ch.MedianX, len(ch.Dots), len(ch.Ticks), tt.x, tt.ticks)
		}
		for _, d := range ch.Dots {
			if d.X != tt.x || d.Y < tt.l.Top || d.Y > tt.l.Base {
				t.Errorf("%s: dot at (%v, %v), want x %v inside the plot", tt.name, d.X, d.Y, tt.x)
			}
		}
	}
}
