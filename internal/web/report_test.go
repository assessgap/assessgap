package web

import (
	"testing"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/web/views"
)

// TestSubjectRateOneRounding: a subject rate on a half cent must print the
// same in the headline, its comparison row and the chart. 19,305 / 1,000 sq ft =
// $19.305, which %.2f prints as $19.30 (binary 19.305 is just below the half);
// Stats carries it already at the cent, half away from zero: $19.31.
func TestSubjectRateOneRounding(t *testing.T) {
	s := comps.Home{PIN: "13331030130000", Class: "203", AV: 25305, BldgAV: 19305, LandAV: 6000,
		BldgAVPerSF: 19.305, BldgSF: 1000, NumCards: 1, HasSplit: true}
	r := &comps.Report{PIN: s.PIN, Year: 2026, AVStage: "mailed", Class: "203", Subject: &s,
		Stats: &comps.Stats{NumComps: 3, SubjectBldgAVPerSF: 19.31, MedianBldgAVPerSF: 18, PctAboveMedian: 0.0725}}
	for i := range 3 {
		r.Comps = append(r.Comps, comps.Comp{Home: comps.Home{PIN: string(rune('a' + i)), BldgSF: 1000, BldgAVPerSF: 18}})
	}
	v := toView(r)
	if v.SubjectBldgAVPerSF != "$19.30/sq ft" || v.Subject.PerSF != "$19.30" || v.Chart.SubjectLbl != "Your home $19.30/sq ft" {
		t.Errorf("subject rate: headline %q, row %q, chart %q; want $19.30 in all three",
			v.SubjectBldgAVPerSF, v.Subject.PerSF, v.Chart.SubjectLbl)
	}
}

// TestChartIdenticalValues covers the degenerate chart where every home has
// the same AV per sq ft: the range is padded to $24–$26 instead of dividing
// by zero, and 20 dots at one x stack into at most chartRows rows.
func TestChartIdenticalValues(t *testing.T) {
	s := comps.Home{PIN: "S", BldgAVPerSF: 25}
	var cs []comps.Comp
	for i := range 20 {
		cs = append(cs, comps.Comp{Home: comps.Home{PIN: string(rune('a' + i)), BldgAVPerSF: 25}})
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
