package web

import (
	"context"
	"encoding/json"
	"testing"

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
