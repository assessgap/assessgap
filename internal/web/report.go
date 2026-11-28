package web

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/format"
	"github.com/assessgap/assessgap/internal/web/views"
)

// toView formats a comps.Report for the report page.
func toView(r *comps.Report) views.ReportData {
	v := views.ReportData{
		PIN: r.PIN, PINFmt: format.PIN(r.PIN), Address: format.TitleCase(r.Address),
		Year: r.Year, Unsupported: r.Unsupported, IsCondo: r.Class == "299",
		Mailed: r.AVStage != "certified", Disclaimer: r.Disclaimer,
	}
	v.Location = location(r)
	v.Window = window(r.Township, time.Now())
	v.StageLabel = stageLabel(r)
	for _, n := range r.NbhdAppeals {
		row := &views.NbhdRow{WinRate: format.Pct(n.WinRate), AvgCut: format.Pct(n.AvgCutPct), Appeals: n.Appeals,
			Years: fmt.Sprintf("%d–%d", n.FirstYear, n.LastYear)}
		if n.ProSe {
			v.ProSe = row
		} else {
			v.Attorney = row
		}
	}
	v.SmallSample = v.ProSe == nil || v.ProSe.Appeals < 20
	if r.Subject == nil {
		return v
	}
	s, st := *r.Subject, r.Stats
	v.Verdict, v.Reasons = r.Verdict, r.Reasons
	v.Headline, v.Badge = headline(r), badge(r)
	// The subject's rate unrounded, as its comparison row and the chart label print
	// it: st.SubjectBldgAVPerSF is already cut to the cent, so at a half cent
	// the headline printed one cent more than those two (the PDF had the same split).
	v.SubjectBldgAVPerSF, v.MedianBldgAVPerSF = perSF(s.BldgAVPerSF), perSF(st.MedianBldgAVPerSF)
	v.GapLabel = format.Gap(st.PctAboveMedian, comps.InLine(st.PctAboveMedian))
	v.NumComps = st.NumComps
	if r.Search != nil {
		v.SearchDesc = r.Search.Description
	}
	v.SubjectAV, v.UniformityAV = format.Money(s.AV), format.Money(st.UniformityAV)
	v.SubjectBldgAV, v.SubjectLandAV, v.LandNote = format.Money(s.BldgAV), format.Money(s.LandAV), comps.LandNote
	// Shown only when the reasons state it too (1% or more above the median).
	if st.StatesReduction() {
		v.Reduction = format.Money(st.PotentialAVReduction)
	}
	v.Subject = homeRow(comps.Comp{Home: s, Miles: 0}, false)
	v.Subject.Away = ""
	ev := map[string]bool{}
	for _, c := range r.Evidence {
		ev[c.PIN] = true
		v.Evidence = append(v.Evidence, homeRow(c, true))
	}
	for _, c := range r.Comps {
		v.Comps = append(v.Comps, homeRow(c, ev[c.PIN]))
	}
	for _, fl := range r.Flags {
		v.Flags = append(v.Flags, fl.Message)
	}
	for _, sa := range r.Sales {
		v.Sales = append(v.Sales, views.SaleRow{Date: shortDate(sa.Date), Price: format.Money(sa.Price)})
	}
	v.Chart = newChart(s, r.Comps, r.Evidence, st.MedianBldgAVPerSF, wideChart)
	v.ChartNarrow = newChart(s, r.Comps, r.Evidence, st.MedianBldgAVPerSF, narrowChart)
	return v
}

func homeRow(c comps.Comp, evidence bool) views.HomeRow {
	h := c.Home
	yr := ""
	if h.YearBuilt > 0 {
		yr = fmt.Sprint(h.YearBuilt)
	}
	land := ""
	if h.LandSF > 0 {
		land = format.Num(h.LandSF)
	}
	f := h.Features()
	return views.HomeRow{
		Address: format.TitleCase(h.Address), PIN: h.PIN, PINFmt: format.PIN(h.PIN),
		Away: format.Miles(c.Miles, ""), Class: h.Class, Stories: f.Stories, Construction: f.Construction,
		SqFt: format.Num(h.BldgSF), YearBuilt: yr, Beds: fmt.Sprint(h.Beds), Baths: format.Baths(h.Baths()),
		Basement: f.Basement, Air: f.Air, Fireplaces: f.Fireplaces, Garage: f.Garage,
		LandSqFt: land, BldgAV: format.Money(h.BldgAV), LandAV: format.Money(h.LandAV), AV: format.Money(h.AV),
		PerSF: format.PerSF(h.BldgAVPerSF), Differences: c.Differences, Evidence: evidence,
	}
}

// headline states the comparison as a fact, with no verdict.
func headline(r *comps.Report) string {
	st := r.Stats
	switch {
	case st == nil || st.TooFewToCompare():
		return "We found too few similar homes to compare."
	case comps.InLine(st.PctAboveMedian):
		return "Your home's building is assessed about the same per square foot as similar homes nearby."
	case st.PctAboveMedian > 0:
		return "Your home's building is assessed " + format.Pct(st.PctAboveMedian) + " above similar homes nearby, per square foot."
	default:
		return "Your home's building is assessed " + format.Pct(math.Abs(st.PctAboveMedian)) + " below similar homes nearby, per square foot."
	}
}

// badge is the short fact shown above the headline.
func badge(r *comps.Report) string {
	if r.Stats == nil || r.Stats.TooFewToCompare() {
		return "Too few similar homes"
	}
	return fmt.Sprintf("%d of %d similar homes have a building assessed lower per sq ft",
		comps.CountBelow(*r.Subject, r.Comps), r.Stats.NumComps)
}

func location(r *comps.Report) string {
	var parts []string
	if r.City != "" {
		parts = append(parts, format.TitleCase(r.City))
	}
	if r.Township != "" {
		parts = append(parts, r.Township+" Township")
	}
	return strings.Join(parts, " · ")
}

// stageLabel explains which assessed value we used.
func stageLabel(r *comps.Report) string {
	if r.AVStage == "certified" {
		return fmt.Sprintf("Uses the Assessor's certified %d values, the ones the Board of Review reviews.", r.Year)
	}
	return fmt.Sprintf("Uses the %d values the Assessor mailed. The Assessor hasn't certified this township yet, "+
		"and certified values can change after Assessor appeals. We update our copy as the county publishes new data.", r.Year)
}

// Chart layouts. The wide one is shown from the sm breakpoint up; the narrow
// one is drawn at phone scale so dots and labels stay legible.
var (
	wideChart   = views.ChartLayout{W: 1000, H: 140, Left: 40, Right: 960, Base: 104, Top: 30, DotR: 6, RowGap: 13, Font: 20, Ticks: 5}
	narrowChart = views.ChartLayout{W: 360, H: 160, Left: 16, Right: 344, Base: 118, Top: 34, DotR: 5, RowGap: 11, Font: 12, Ticks: 3}
)

// chartRows dots can stack at one x; beyond that they overlap in the top row.
const chartRows = 5

func newChart(s comps.Home, cs []comps.Comp, ev []comps.Comp, median float64, l views.ChartLayout) views.Chart {
	lo, hi := s.BldgAVPerSF, s.BldgAVPerSF
	for _, c := range cs {
		lo, hi = math.Min(lo, c.BldgAVPerSF), math.Max(hi, c.BldgAVPerSF)
	}
	pad := (hi - lo) * 0.08
	if pad == 0 {
		pad = 1
	}
	lo, hi = math.Max(0, lo-pad), hi+pad
	x := func(v float64) float64 { return l.Left + (v-lo)/(hi-lo)*(l.Right-l.Left) }
	rowY := func(row int) float64 { return l.Base - l.DotR*2 - float64(row)*l.RowGap }

	isEv := map[string]bool{}
	for _, e := range ev {
		isEv[e.PIN] = true
	}
	ch := views.Chart{
		L:          l,
		SubjectX:   x(s.BldgAVPerSF),
		MedianX:    x(median),
		SubjectLbl: "Your home " + perSF(s.BldgAVPerSF),
		MedLbl:     "Median " + perSF(median),
		MedAnchor:  "middle",
	}
	switch mx := ch.MedianX; {
	case mx < l.Left+(l.Right-l.Left)*0.2:
		ch.MedAnchor = "start"
	case mx > l.Left+(l.Right-l.Left)*0.8:
		ch.MedAnchor = "end"
	}
	// Stack dots that would overlap into rows so each stays visible.
	minDX := l.DotR * 2.3
	for _, c := range cs {
		d := views.Dot{X: x(c.BldgAVPerSF), Evidence: isEv[c.PIN],
			Title: fmt.Sprintf("%s: %s", format.TitleCase(c.Address), perSF(c.BldgAVPerSF))}
		row := 0
		for ; row < chartRows-1; row++ {
			if !occupied(ch.Dots, d.X, rowY(row), minDX) {
				break
			}
		}
		d.Y = rowY(row)
		ch.Dots = append(ch.Dots, d)
	}
	step := niceStep((hi - lo) / float64(l.Ticks))
	for t := math.Ceil(lo/step) * step; t <= hi; t += step {
		ch.Ticks = append(ch.Ticks, views.Tick{X: x(t), Label: perSFShort(t)})
	}
	return ch
}

func occupied(ds []views.Dot, x, y, minDX float64) bool {
	for _, d := range ds {
		if d.Y == y && math.Abs(d.X-x) < minDX {
			return true
		}
	}
	return false
}

func niceStep(raw float64) float64 {
	if raw <= 0 {
		return 1
	}
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if raw <= m*mag {
			return m * mag
		}
	}
	return 10 * mag
}

// ---- formatting

// perSF is a rate with its unit, for the headline and the chart labels.
func perSF(v float64) string { return format.PerSF(v) + "/sq ft" }

// perSFShort is a chart tick: whole dollars, or one decimal.
func perSFShort(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("$%.0f", v)
	}
	return fmt.Sprintf("$%.1f", v)
}

func shortDate(d string) string {
	if len(d) >= 7 {
		return d[:7]
	}
	return d
}
