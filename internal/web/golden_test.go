package web

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/fixture"
	"github.com/assessgap/assessgap/internal/golden"
	"github.com/assessgap/assessgap/internal/web/views"
)

// viewCases are the report shapes whose page data is goldened, in
// testdata/golden/web/<name>.txt. Names match internal/golden.Cases.
var viewCases = map[string]bool{
	"well-above-mailed": true, "slightly-above": true, "in-line": true, "below-none-lower": true,
	"too-few-comps": true, "check-no-full-baths": true, "sales-with-deed-types": true, "unsupported-condo": true,
}

// TestGoldenReportView compares the report page's formatted figures with
// testdata/golden/web. The filing window (which depends on today's date) and
// the chart geometry are left out; the chart's labels are kept.
func TestGoldenReportView(t *testing.T) {
	e := comps.New(fixture.Open(t))
	found := 0
	for _, c := range golden.Cases {
		if viewCases[c.Name] {
			found++
		}
	}
	if found != len(viewCases) {
		t.Fatalf("%d of %d viewCases are not in golden.Cases (renamed?)", len(viewCases)-found, len(viewCases))
	}
	for _, c := range golden.Cases {
		if !viewCases[c.Name] {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			r, err := e.Report(context.Background(), c.Args[0], 0)
			if err != nil {
				t.Fatal(err)
			}
			v := toView(r)
			type labels struct{ Subject, Median string }
			proj := struct {
				views.ReportData
				Window, Chart, ChartNarrow struct{}
				ChartLabels                labels
			}{ReportData: v, ChartLabels: labels{v.Chart.SubjectLbl, v.Chart.MedLbl}}
			b, err := json.Marshal(proj)
			if err != nil {
				t.Fatal(err)
			}
			got, err := golden.Flatten(b)
			if err != nil {
				t.Fatal(err)
			}
			header := "Report page data (web.toView) for PIN " + c.Args[0] +
				"\nRegenerate deliberately: go test ./internal/golden ./internal/web -run Golden -update"
			golden.CheckFields(t, "web/"+c.Name+".txt", c.Name+" ("+c.Args[0]+")", header, got)
		})
	}
}

// TestGoldenTownships compares the township pages' data on fixed dates with
// testdata/golden/web/township*.txt: the filing window (from internal/bor)
// and the figures aggregated from the fixture's neighborhoods. Dates are
// noon in Chicago.
func TestGoldenTownships(t *testing.T) {
	facts, err := loadTownshipFacts(context.Background(), fixture.Open(t))
	if err != nil {
		t.Fatal(err)
	}
	day := func(s string) time.Time {
		d, err := time.Parse(time.DateOnly, s)
		if err != nil {
			t.Fatal(err)
		}
		return d.Add(18 * time.Hour) // noon in Chicago (UTC−6 in winter, −5 in summer: still that day)
	}
	header := "Township page data (web.townshipView, web.townshipIndex) on a fixed date" +
		"\nRegenerate deliberately: go test ./internal/golden ./internal/web -run Golden -update"
	check := func(name string, v any) {
		t.Helper()
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		got, err := golden.Flatten(b)
		if err != nil {
			t.Fatal(err)
		}
		golden.CheckFields(t, "web/"+name+".txt", name, header, got)
	}
	for _, c := range []struct{ township, on string }{
		{"Oak Park", "2026-08-20"},  // open; in the fixture
		{"Worth", "2026-09-15"},     // dates not posted; in the fixture
		{"Jefferson", "2026-09-15"}, // dates not posted; in the fixture
		{"Hyde Park", "2026-09-15"}, // opening soon; no data in the fixture
	} {
		v := townshipView(c.township, facts[c.township], day(c.on))
		check("township-"+v.Slug+"-"+c.on, v)
	}
	check("townships-index-2026-09-15", townshipIndex(day("2026-09-15")))
}
