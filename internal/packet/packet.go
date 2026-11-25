// Package packet renders a printable filing packet (PDF) from a comps report:
// a summary page, a comparable-property grid, and a filing checklist.
//
// Like the rest of the product, it states facts about assessed values only.
// It never states or estimates market value and never tells the homeowner
// what to argue.
package packet

import (
	"errors"
	"fmt"
	"github.com/assessgap/assessgap/internal/bor"
	"io"
	"math"
	"strings"
	"time"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/dustin/go-humanize"
)

// NotAffiliated goes on every page.
const NotAffiliated = "AssessGap is not affiliated with Cook County. You file and sign your own appeal."

// Pages is the number of pages Render produces.
const Pages = 3

// ErrUnsupported means the report has no comparison to print.
var ErrUnsupported = errors.New("packet: report has no comparison")

const (
	margin   = 54.0 // 0.75 in
	contentW = pageW - 2*margin
	bodySize = 10.5
	leading  = 14.5
)

// Render writes the packet for r. generated is printed as the preparation date.
func Render(w io.Writer, r *comps.Report, generated time.Time) error {
	d, err := build(r, generated)
	if err != nil {
		return err
	}
	return d.writeTo(w)
}

func build(r *comps.Report, generated time.Time) (*doc, error) {
	if r == nil || r.Subject == nil || r.Stats == nil {
		reason := "no subject"
		if r != nil && r.Unsupported != "" {
			reason = r.Unsupported
		}
		return nil, fmt.Errorf("%w: %s", ErrUnsupported, reason)
	}
	p := &pager{doc: newDoc(fmt.Sprintf("Assessment comparison, PIN %s, tax year %d", formatPIN(r.PIN), r.Year)), r: r}
	for _, page := range []func(){func() { p.summary(generated) }, p.grid, func() { p.steps(generated) }} {
		page()
		if p.y > bottom {
			return nil, fmt.Errorf("packet: page %d content runs into the footer (y=%.0f > %.0f)", p.page, p.y, bottom)
		}
	}
	return p.doc, nil
}

// bottom is the lowest y body content may reach (the footer sits below it).
const bottom = pageH - margin - 12

// Text returns every string drawn in the packet, in order, one per line.
// It is meant for checks such as banned-word tests.
func Text(r *comps.Report, generated time.Time) (string, error) {
	d, err := build(r, generated)
	if err != nil {
		return "", err
	}
	return strings.Join(d.texts, "\n"), nil
}

// pager tracks the write position on the current page.
type pager struct {
	*doc
	r    *comps.Report
	y    float64
	page int
}

func (p *pager) newPage(title string) {
	p.addPage()
	p.page++
	p.y = margin + 14
	p.text(margin, p.y, bold, 16, title)
	p.y += 8
	p.line(margin, p.y, pageW-margin, p.y, 1)
	p.y += 20
	p.footer()
}

func (p *pager) footer() {
	y := pageH - margin + 6
	p.line(margin, y-12, pageW-margin, y-12, 0.5)
	p.text(margin, y, regular, 8, fmt.Sprintf("PIN %s · Tax year %d", formatPIN(p.r.PIN), p.r.Year))
	p.textRight(pageW-margin, y, regular, 8, fmt.Sprintf("Page %d of %d", p.page, Pages))
	p.text(margin, y+11, regular, 8, NotAffiliated)
}

func (p *pager) heading(s string) {
	p.y += 6
	p.text(margin, p.y, bold, 12, s)
	p.y += leading + 2
}

// para writes wrapped text at x with width w and advances the cursor.
func (p *pager) para(x, w float64, f font, size float64, s string) {
	lead := size * 1.38
	for _, ln := range wrap(f, size, w, s) {
		p.text(x, p.y, f, size, ln)
		p.y += lead
	}
	p.y += 4
}

func (p *pager) body(s string) { p.para(margin, contentW, regular, bodySize, s) }

// ---------------------------------------------------------------- page 1

func (p *pager) summary(generated time.Time) {
	r, s, st := p.r, p.r.Subject, p.r.Stats
	p.newPage(fmt.Sprintf("Assessment comparison: tax year %d", r.Year))
	p.para(margin, contentW, regular, 9, fmt.Sprintf(
		"Prepared %s from public Cook County Assessor data. Figures are assessed values (AV) as published by the Assessor.",
		generated.Format("January 2, 2006")))

	p.heading("Property")
	stage := "Certified: the Assessor's value after Assessor appeals"
	if r.AVStage != "certified" {
		stage = "Mailed: the Assessor's initial value; not yet certified, and it may change"
	}
	addr := titleCase(s.Address)
	if r.City != "" {
		addr += ", " + titleCase(r.City)
	}
	rows := [][2]string{
		{"Address", addr},
		{"PIN", formatPIN(r.PIN)},
		{"Property class", r.Class},
		{"Township", r.Township},
		{"Tax year", fmt.Sprint(r.Year)},
		{"Assessed value", money(s.AV)},
		{"Value stage", stage},
		{"Building", fmt.Sprintf("%s sq ft, built %s, %s, %s", num(s.BldgSF), yearOr(s.YearBuilt),
			plural(fmt.Sprint(s.Beds), "bedroom"), plural(baths(*s), "bath"))},
	}
	p.keyValues(rows)

	p.heading("Comparison with similar homes")
	p.body(fmt.Sprintf(
		"The property is assessed at %s per square foot of building area. The median of %d similar homes "+
			"in the same Assessor neighborhood is %s per square foot, so the property is %s the median.",
		perSF(s.AVPerSF), st.NumComps, perSF(st.MedianAVPerSF), gapPhrase(st.PctAboveMedian)))
	below := len(p.belowSubject())
	p.body(fmt.Sprintf("%d of the %d comparable homes are assessed lower per square foot than the property. "+
		"The property ranks at the %s percentile of the comparable homes by AV per square foot.",
		below, st.NumComps, ordinal(int(math.Round(st.SubjectPercentile)))))
	if r.Search != nil {
		p.body(fmt.Sprintf("How comparable homes were chosen: %s (Assessor neighborhood %s), excluding multi-building "+
			"parcels and homes without recorded square footage. The %d most similar by size, age, baths, land and "+
			"construction are listed on page 2.", r.Search.Description, r.Nbhd, st.NumComps))
	}

	p.heading("Uniformity figure")
	if st.PotentialAVReduction >= 1 {
		p.body(fmt.Sprintf(
			"Applying the comparable homes' median of %s per square foot to the property's %s square feet gives "+
				"an assessed value of %s. That is %s less than the current assessed value of %s.",
			perSF(st.MedianAVPerSF), num(s.BldgSF), money(st.UniformityAV), money(st.PotentialAVReduction), money(s.AV)))
	} else {
		p.body(fmt.Sprintf(
			"Applying the comparable homes' median of %s per square foot to the property's %s square feet gives "+
				"an assessed value of %s, which is not below the current assessed value of %s.",
			perSF(st.MedianAVPerSF), num(s.BldgSF), money(st.UniformityAV), money(s.AV)))
	}

	if len(r.Flags) > 0 {
		p.heading("Please verify")
		for _, f := range r.Flags {
			p.bullet(f.Message)
		}
	}

	p.y += 6
	p.boxed(r.Disclaimer + " " + NotAffiliated)
}

func (p *pager) keyValues(rows [][2]string) {
	const keyW = 105
	for _, kv := range rows {
		p.text(margin, p.y, bold, bodySize, kv[0])
		lines := wrap(regular, bodySize, contentW-keyW, kv[1])
		for i, ln := range lines {
			p.text(margin+keyW, p.y, regular, bodySize, ln)
			if i < len(lines)-1 {
				p.y += leading
			}
		}
		p.y += leading
	}
	p.y += 4
}

func (p *pager) bullet(s string) {
	p.text(margin+4, p.y, regular, bodySize, "•")
	p.para(margin+16, contentW-16, regular, bodySize, s)
}

// boxed writes s inside a light gray bordered box.
func (p *pager) boxed(s string) {
	const pad, size = 8.0, 9.0
	lines := wrap(regular, size, contentW-2*pad, s)
	h := float64(len(lines))*size*1.38 + 2*pad - 2
	p.fillRect(margin, p.y, contentW, h, 0.95)
	p.strokeRect(margin, p.y, contentW, h, 0.5)
	y := p.y + pad + size
	for _, ln := range lines {
		p.text(margin+pad, y, regular, size, ln)
		y += size * 1.38
	}
	p.y += h + 8
}

func (p *pager) belowSubject() []comps.Comp {
	var out []comps.Comp
	for _, c := range p.r.Comps {
		if c.AVPerSF < p.r.Subject.AVPerSF {
			out = append(out, c)
		}
	}
	return out
}

// ---------------------------------------------------------------- page 2

type column struct {
	title string
	w     float64
	right bool
	val   func(comps.Home) string
}

var columns = []column{
	{"PIN", 82, false, func(h comps.Home) string { return formatPIN(h.PIN) }},
	{"Address", 126, false, func(h comps.Home) string { return titleCase(h.Address) }},
	{"Class", 30, false, func(h comps.Home) string { return h.Class }},
	{"Bldg sq ft", 48, true, func(h comps.Home) string { return num(h.BldgSF) }},
	{"Built", 32, true, func(h comps.Home) string { return yearOr(h.YearBuilt) }},
	{"Beds/baths", 58, true, func(h comps.Home) string { return fmt.Sprintf("%d / %s", h.Beds, baths(h)) }},
	{"Assessed value", 70, true, func(h comps.Home) string { return money(h.AV) }},
	{"AV / sq ft", 58, true, func(h comps.Home) string { return fmt.Sprintf("$%.2f", h.AVPerSF) }},
}

const (
	cellSize = 8.0
	rowH     = 15.0
	cellPad  = 4.0
)

func (p *pager) grid() {
	r := p.r
	p.newPage("Comparable property grid")
	p.para(margin, contentW, regular, 9.5, fmt.Sprintf(
		"Tax year %d assessed values from the Cook County Assessor, %s stage. Building characteristics are the "+
			"Assessor's records. Evidence comparables are the most similar homes assessed lower per square foot "+
			"than the subject. AV / sq ft = assessed value ÷ building square feet.", r.Year, r.AVStage))
	p.y += 2

	tableW := 0.0
	for _, c := range columns {
		tableW += c.w
	}
	x0 := margin + (contentW-tableW)/2

	top := p.y
	p.cells(x0, rowH+4, bold, 0.82, func(c column) string { return c.title })
	p.section(x0, tableW, "Subject property")
	p.row(x0, *r.Subject, bold, 0.92)
	ev := map[string]bool{}
	p.section(x0, tableW, fmt.Sprintf("Evidence comparables (%d): similar homes assessed lower per sq ft", len(r.Evidence)))
	for _, c := range r.Evidence {
		ev[c.PIN] = true
		p.row(x0, c.Home, regular, -1)
	}
	var rest []comps.Comp
	for _, c := range r.Comps {
		if !ev[c.PIN] {
			rest = append(rest, c)
		}
	}
	if len(rest) > 0 {
		p.section(x0, tableW, fmt.Sprintf("Other comparable homes (%d)", len(rest)))
		for _, c := range rest {
			p.row(x0, c.Home, regular, -1)
		}
	}
	p.strokeRect(x0, top, tableW, p.y-top, 0.8)

	p.y += 14
	st := r.Stats
	p.para(margin, contentW, bold, 9.5, fmt.Sprintf("Median AV / sq ft of the %d comparable homes: $%.2f.  Subject: $%.2f (%s the median).",
		st.NumComps, st.MedianAVPerSF, st.SubjectAVPerSF, gapPhrase(st.PctAboveMedian)))
	if r.Search != nil {
		p.para(margin, contentW, regular, 9, "Selection: "+r.Search.Description+" (Assessor neighborhood "+r.Nbhd+
			"). Multi-building parcels and homes without recorded square footage are excluded.")
	}
}

func (p *pager) row(x0 float64, h comps.Home, f font, gray float64) {
	p.cells(x0, rowH, f, gray, func(c column) string { return c.val(h) })
}

// cells draws one table row of height h with column rules and a bottom rule;
// gray < 0 means no fill.
func (p *pager) cells(x0, h float64, f font, gray float64, val func(column) string) {
	x := x0
	for i, c := range columns {
		if gray >= 0 {
			p.fillRect(x, p.y, c.w, h, gray)
		}
		if i > 0 {
			p.line(x, p.y, x, p.y+h, 0.4)
		}
		p.cell(x, p.y, c.w, h, f, val(c), c.right)
		x += c.w
	}
	p.y += h
	p.line(x0, p.y, x, p.y, 0.4)
}

// section draws a full-width label row.
func (p *pager) section(x0, w float64, title string) {
	p.fillRect(x0, p.y, w, rowH, 0.96)
	p.text(x0+cellPad, p.y+rowH-4.5, bold, cellSize, title)
	p.y += rowH
	p.line(x0, p.y, x0+w, p.y, 0.4)
}

// cell writes s in a cell, truncating it to fit.
func (p *pager) cell(x, y, w, h float64, f font, s string, right bool) {
	s = fit(f, cellSize, w-2*cellPad, s)
	base := y + h - 4.5
	if right {
		p.textRight(x+w-cellPad, base, f, cellSize, s)
	} else {
		p.text(x+cellPad, base, f, cellSize, s)
	}
}

func fit(f font, size, w float64, s string) string {
	if textWidth(f, size, s) <= w {
		return s
	}
	for len(s) > 0 && textWidth(f, size, s+"...") > w {
		s = s[:len(s)-1]
	}
	return strings.TrimSpace(s) + "..."
}

// ---------------------------------------------------------------- page 3

func (p *pager) steps(now time.Time) {
	r := p.r
	p.newPage("Filing steps: Cook County Board of Review")
	p.para(margin, contentW, regular, 9.5,
		"How a homeowner files their own appeal, from the Board of Review's published instructions and rules (checked "+
			bor.ScheduleAsOf+"). Rules and dates change; the Board's website is the final word. This is general "+
			"information, not legal advice. You file, sign and present your own appeal.")
	p.y += 4
	steps := []string{
		windowStep(r.Township, now),
		"Take a color photo of the front of your home. The Board's Rule 14 requires one, taken within the last 12 " +
			"months, in the brief you upload. Mark the date it was taken. Photos of the comparable homes are encouraged.",
		"Check the details on page 2 (square feet, year built, bedrooms, baths) against your home. If the Assessor " +
			"has something wrong, say so in your brief.",
		"Go to " + bor.PortalURL + " and choose Submit an Appeal. You can create an account or continue as a guest " +
			"(guests get the decision by mail only; account holders also get it by email).",
		"Choose \"" + bor.AppealType + "\" as the type of appeal and enter PIN " + formatPIN(r.PIN) + ". When asked if " +
			"you are a Board of Review attorney, answer No. Choose your appellant type (owner).",
		"Decide whether you want a hearing. It is optional, held by phone for homeowners, and the Board says you are " +
			"not penalized for skipping it. You choose this when you file.",
		"Enter comparable properties if you like (optional: the Board's analysts also pull their own). The PINs on " +
			"page 2 are similar homes assessed lower per square foot than yours, from the same neighborhood and class.",
		"Review, initial, confirm and submit before the deadline. Save your complaint number.",
		"Upload your evidence after submitting: your brief with the photo, and this report if you wish, by the " +
			"evidence deadline for your township.",
		fmt.Sprintf("Watch for the decision, usually %s weeks after your township closes. You can check status at %s. "+
			"A re-review must be requested within %d days of the decision letter. If you disagree with the decision, "+
			"you can appeal to the state Property Tax Appeal Board within %d days, and you can appeal again next year.",
			bor.DecisionWeeks, bor.DecisionsURL, bor.ReReviewDays, bor.PTABDays),
	}
	for i, s := range steps {
		p.checkItem(i+1, s)
	}
	p.y += 8
	p.boxed(NotAffiliated + " " + r.Disclaimer)
}

// windowStep describes the township's filing window at time now.
func windowStep(township string, now time.Time) string {
	st := bor.StatusOf(township, now)
	day := func(t time.Time) string { return t.Format("January 2, 2006") }
	switch st.State {
	case bor.Open:
		return fmt.Sprintf("File by %s. %s Township is open for Board of Review appeals now. Evidence is due by %s.",
			day(st.Group.Close), township, day(st.Group.EvidenceDue))
	case bor.Upcoming:
		return fmt.Sprintf("%s Township opens for Board of Review appeals on %s and closes on %s. Evidence is due by %s.",
			township, day(st.Group.Open), day(st.Group.Close), day(st.Group.EvidenceDue))
	case bor.Closed:
		return fmt.Sprintf("%s Township's %d Board of Review window closed on %s. Check %s for next year's dates.",
			township, bor.SessionYear, day(st.Group.Close), bor.DatesURL)
	default:
		return fmt.Sprintf("Check when %s Township opens. The Board hasn't posted its dates yet; each township is open "+
			"about 30 days. You can pre-register on the Board's portal now. Dates: %s", township, bor.DatesURL)
	}
}

func (p *pager) checkItem(n int, s string) {
	const box, indent = 9.0, 38.0
	p.strokeRect(margin, p.y-box+1, box, box, 0.7)
	p.text(margin+box+6, p.y, bold, bodySize, fmt.Sprintf("%d.", n))
	p.para(margin+indent, contentW-indent, regular, bodySize, s)
	p.y += 2
}

// ---------------------------------------------------------------- formatting
// (mirrors internal/web/report.go; kept local so the packet doesn't depend on the web package)

func money(v float64) string { return "$" + humanize.Comma(int64(math.Round(v))) }

func num(v float64) string { return humanize.Comma(int64(math.Round(v))) }

func perSF(v float64) string { return fmt.Sprintf("$%.2f", v) }

func gapPhrase(g float64) string {
	switch {
	case g >= 0.005:
		return fmt.Sprintf("%.0f%% above", 100*g)
	case g <= -0.005:
		return fmt.Sprintf("%.0f%% below", -100*g)
	default:
		return "about equal to"
	}
}

func baths(h comps.Home) string {
	b := h.Baths()
	if b == math.Trunc(b) {
		return fmt.Sprintf("%.0f", b)
	}
	return fmt.Sprintf("%.1f", b)
}

func plural(n, noun string) string {
	if n == "1" {
		return n + " " + noun
	}
	return n + " " + noun + "s"
}

func yearOr(y int) string {
	if y <= 0 {
		return "unknown"
	}
	return fmt.Sprint(y)
}

func ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

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

func formatPIN(p string) string {
	if len(p) != 14 {
		return p
	}
	return p[0:2] + "-" + p[2:4] + "-" + p[4:7] + "-" + p[7:10] + "-" + p[10:14]
}
