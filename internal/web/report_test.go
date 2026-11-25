package web

import (
	"testing"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/web/views"
)

func TestChartIdenticalComps(t *testing.T) {
	s := comps.Home{PIN: "S", AVPerSF: 30}
	var cs []comps.Comp
	for i := range 20 {
		cs = append(cs, comps.Comp{Home: comps.Home{PIN: string(rune('a' + i)), AVPerSF: 25}})
	}
	for _, l := range []views.ChartLayout{wideChart, narrowChart} {
		ch := newChart(s, cs, nil, 25, l) // must terminate
		if len(ch.Dots) != 20 {
			t.Fatalf("%d dots", len(ch.Dots))
		}
		for _, d := range ch.Dots {
			if d.Y < l.Top || d.Y > l.Base {
				t.Errorf("dot outside plot area: y=%v", d.Y)
			}
		}
		if len(ch.Ticks) == 0 {
			t.Error("no ticks")
		}
	}
}
