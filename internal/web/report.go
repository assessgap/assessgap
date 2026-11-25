package web

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/assessgap/assessgap/internal/bor"
	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/search"
	"github.com/assessgap/assessgap/internal/web/views"
	"github.com/dustin/go-humanize"
)

// toView formats a comps.Report for the report page.
func toView(r *comps.Report) views.ReportData {
	v := views.ReportData{
		PIN: r.PIN, PINFmt: formatPIN(r.PIN), Address: titleCase(r.Address),
		Year: r.Year, Unsupported: r.Unsupported, IsCondo: r.Class == "299",
		Mailed: r.AVStage != "certified", Disclaimer: r.Disclaimer,
	}
	v.Location = location(r)
	v.Window = window(r.Township, time.Now())
	v.StageLabel = stageLabel(r)
	for _, n := range r.NbhdAppeals {
		row := &views.NbhdRow{WinRate: pct(n.WinRate), AvgCut: pct(n.AvgCutPct), Appeals: n.Appeals,
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
	v.SubjectAVPerSF, v.MedianAVPerSF = perSF(st.SubjectAVPerSF), perSF(st.MedianAVPerSF)
	switch {
	case st.PctAboveMedian >= 0.005:
		v.GapLabel = pct(st.PctAboveMedian) + " above"
	case st.PctAboveMedian <= -0.005:
		v.GapLabel = pctAbs(st.PctAboveMedian) + " below"
	default:
		v.GapLabel = "about equal to"
	}
	v.NumComps = st.NumComps
	if r.Search != nil {
		v.SearchDesc = r.Search.Description
	}
	v.SubjectAV, v.UniformityAV = money(s.AV), money(st.UniformityAV)
	if st.PotentialAVReduction >= 1 {
		v.Reduction = money(st.PotentialAVReduction)
	}
	v.Subject = homeRow(s, false)
	ev := map[string]bool{}
	for _, c := range r.Evidence {
		ev[c.PIN] = true
		v.Evidence = append(v.Evidence, homeRow(c.Home, true))
	}
	for _, c := range r.Comps {
		v.Comps = append(v.Comps, homeRow(c.Home, ev[c.PIN]))
	}
	for _, fl := range r.Flags {
		v.Flags = append(v.Flags, fl.Message)
	}
	for _, sa := range r.Sales {
		v.Sales = append(v.Sales, views.SaleRow{Date: shortDate(sa.Date), Price: money(sa.Price)})
	}
	v.Chart = newChart(s, r.Comps, r.Evidence, st.MedianAVPerSF, wideChart)
	v.ChartNarrow = newChart(s, r.Comps, r.Evidence, st.MedianAVPerSF, narrowChart)
	return v
}

func homeRow(h comps.Home, evidence bool) views.HomeRow {
	yr := ""
	if h.YearBuilt > 0 {
		yr = fmt.Sprint(h.YearBuilt)
	}
	return views.HomeRow{
		Address: titleCase(h.Address), PIN: h.PIN, PINFmt: formatPIN(h.PIN),
		SqFt: num(h.BldgSF), YearBuilt: yr, Beds: fmt.Sprint(h.Beds), Baths: baths(h),
		AV: money(h.AV), PerSF: fmt.Sprintf("$%.2f", h.AVPerSF), Evidence: evidence,
	}
}

// headline states the comparison as a fact, with no verdict.
func headline(r *comps.Report) string {
	st := r.Stats
	switch {
	case st == nil || st.NumComps < 3:
		return "We found too few similar homes to compare."
	case st.PctAboveMedian >= 0.005:
		return "Your home is assessed " + pct(st.PctAboveMedian) + " above similar homes nearby."
	case st.PctAboveMedian <= -0.005:
		return "Your home is assessed " + pctAbs(st.PctAboveMedian) + " below similar homes nearby."
	default:
		return "Your home is assessed about the same as similar homes nearby."
	}
}

// badge is the short fact shown above the headline.
func badge(r *comps.Report) string {
	if r.Stats == nil || r.Stats.NumComps < 3 {
		return "Too few similar homes"
	}
	lower := 0
	for _, c := range r.Comps {
		if c.AVPerSF < r.Subject.AVPerSF {
			lower++
		}
	}
	return fmt.Sprintf("%d of %d similar homes assessed lower", lower, r.Stats.NumComps)
}

func location(r *comps.Report) string {
	var parts []string
	if r.City != "" {
		parts = append(parts, titleCase(r.City))
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
	lo, hi := s.AVPerSF, s.AVPerSF
	for _, c := range cs {
		lo, hi = math.Min(lo, c.AVPerSF), math.Max(hi, c.AVPerSF)
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
		SubjectX:   x(s.AVPerSF),
		MedianX:    x(median),
		SubjectLbl: "Your home " + perSF(s.AVPerSF),
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
		d := views.Dot{X: x(c.AVPerSF), Evidence: isEv[c.PIN],
			Title: fmt.Sprintf("%s: %s", titleCase(c.Address), perSF(c.AVPerSF))}
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

func money(v float64) string { return "$" + humanize.Comma(int64(math.Round(v))) }

func perSF(v float64) string { return fmt.Sprintf("$%.2f/sq ft", v) }

func perSFShort(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("$%.0f", v)
	}
	return fmt.Sprintf("$%.1f", v)
}

func pct(v float64) string { return fmt.Sprintf("%.0f%%", 100*v) }

func pctAbs(v float64) string { return pct(math.Abs(v)) }

func num(v float64) string { return humanize.Comma(int64(math.Round(v))) }

func f(v float64) string { return fmt.Sprintf("%.1f", v) }

func baths(h comps.Home) string {
	b := h.Baths()
	if b == math.Trunc(b) {
		return fmt.Sprintf("%.0f", b)
	}
	return fmt.Sprintf("%.1f", b)
}

// titleCase renders "5555 W BELMONT AVE" as "5555 W Belmont Ave".
func titleCase(s string) string {
	ws := strings.Fields(strings.ToLower(s))
	for i, w := range ws {
		if len(w) == 1 && strings.Contains("nsew", w) {
			ws[i] = strings.ToUpper(w)
			continue
		}
		ws[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(ws, " ")
}

func formatPIN(p string) string { return search.FormatPIN(p) }

func shortDate(d string) string {
	if len(d) >= 7 {
		return d[:7]
	}
	return d
}

// window describes the township's filing window for the report banner.
func window(township string, now time.Time) views.Window {
	if township == "" {
		return views.Window{}
	}
	st := bor.StatusOf(township, now)
	day := func(t time.Time) string { return t.Format("January 2") }
	t := township + " Township"
	switch st.State {
	case bor.Open:
		left := fmt.Sprintf("%d days left", st.DaysLeft)
		if st.DaysLeft == 1 {
			left = "last day"
		}
		return views.Window{State: "open",
			Text:     fmt.Sprintf("%s is open for Board of Review appeals. File by %s (%s).", t, day(st.Group.Close), left),
			LinkText: "Go to the Board's filing portal", LinkURL: bor.PortalURL}
	case bor.Upcoming:
		return views.Window{State: "upcoming",
			Text:     fmt.Sprintf("%s opens for Board of Review appeals on %s and closes on %s.", t, day(st.Group.Open), day(st.Group.Close)),
			LinkText: "Board of Review dates", LinkURL: bor.DatesURL}
	case bor.Closed:
		return views.Window{State: "closed",
			Text:     fmt.Sprintf("%s's %d Board of Review window closed on %s. Leave your email below and we'll tell you when the next one opens.", t, bor.SessionYear, day(st.Group.Close)),
			LinkText: "Board of Review dates", LinkURL: bor.DatesURL}
	default:
		return views.Window{State: "unscheduled",
			Text:     fmt.Sprintf("The Board of Review hasn't posted %s's dates yet. You can pre-register on the Board's portal now, and we'll email you when it opens if you leave your email below.", t),
			LinkText: "Pre-register with the Board", LinkURL: bor.PortalURL}
	}
}
