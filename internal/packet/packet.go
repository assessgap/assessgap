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
	"io"
	"math"
	"strings"
	"time"

	"github.com/assessgap/assessgap/internal/bor"
	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/format"
)

// NotAffiliated goes on every page.
const NotAffiliated = "AssessGap is not affiliated with Cook County. You file and sign your own appeal."

// Pages is the number of pages Render produces.
const Pages = 3

// ErrUnsupported means the report has no comparison to print.
var ErrUnsupported = errors.New("packet: report has no comparison")

// TooFewReason is why a supported home with fewer than 3 similar homes gets
// no evidence report.
const TooFewReason = "too few similar homes were found to compare"

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
	// With fewer than 3 similar homes every surface says there are too few to
	// compare (comps' decide, the web page, the CLI), so there is no median to
	// print: with none, page 1 would state a $0.00 median.
	if r.Stats.TooFewToCompare() {
		return nil, fmt.Errorf("%w: %s", ErrUnsupported, TooFewReason)
	}
	p := &pager{doc: newDoc(fmt.Sprintf("Assessment comparison, PIN %s, tax year %d", format.PIN(r.PIN), r.Year)), r: r}
	for _, page := range []func(){func() { p.summary(generated) }, p.grid, func() { p.steps(generated) }} {
		page()
		if p.y > p.bottom() {
			return nil, fmt.Errorf("packet: page %d content runs into the footer (y=%.0f > %.0f)", p.page, p.y, p.bottom())
		}
	}
	return p.doc, nil
}

// bottom is the lowest y body content may reach on the current page (the
// footer sits below it).
func (p *pager) bottom() float64 { return p.cur.h - p.m - 12 }

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
	m    float64 // the current page's margin
	rowH float64 // grid row height (page 2)
}

// newPage starts a portrait page with the standard margin.
func (p *pager) newPage(title string) { p.newPageSized(title, pageW, pageH, margin) }

// newPageSized starts a page of size w × h with margin m.
func (p *pager) newPageSized(title string, w, h, m float64) {
	p.addPage(w, h)
	p.page++
	p.m = m
	p.y = m + 14
	p.text(m, p.y, bold, 16, title)
	p.y += 8
	p.line(m, p.y, w-m, p.y, 1)
	p.y += 20
	p.footer()
}

func (p *pager) footer() {
	m, w := p.m, p.cur.w
	y := p.cur.h - m + 6
	p.line(m, y-12, w-m, y-12, 0.5)
	p.text(m, y, regular, 8, fmt.Sprintf("PIN %s · Tax year %d", format.PIN(p.r.PIN), p.r.Year))
	p.textRight(w-m, y, regular, 8, fmt.Sprintf("Page %d of %d", p.page, Pages))
	p.text(m, y+11, regular, 8, NotAffiliated)
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
		format.LongDate(bor.InChicago(generated))))

	p.heading("Property")
	stage := "Certified: the Assessor's value after Assessor appeals"
	if r.AVStage != "certified" {
		stage = "Mailed: the Assessor's initial value; not yet certified, and it may change"
	}
	addr := format.TitleCase(s.Address)
	if r.City != "" {
		addr += ", " + format.TitleCase(r.City)
	}
	rows := [][2]string{
		{"Address", addr},
		{"PIN", format.PIN(r.PIN)},
		{"Property class", r.Class},
		{"Township", r.Township},
		{"Tax year", fmt.Sprint(r.Year)},
		{"Assessed value", fmt.Sprintf("%s (building %s + land %s)", format.Money(s.AV), format.Money(s.BldgAV), format.Money(s.LandAV))},
		{"Value stage", stage},
		{"Building", fmt.Sprintf("%s sq ft, built %s, %s, %s", format.Num(s.BldgSF), yearOr(s.YearBuilt),
			plural(fmt.Sprint(s.Beds), "bedroom"), plural(format.Baths(s.Baths()), "bath"))},
	}
	p.keyValues(rows)

	p.heading("Comparison with similar homes")
	p.body(fmt.Sprintf(
		"The property's building is assessed at %s per square foot of building area (building AV %s ÷ %s sq ft). "+
			"The median of %d similar homes in the same Assessor neighborhood is %s per square foot, so the property is %s the median.",
		format.PerSF(s.BldgAVPerSF), format.Money(s.BldgAV), format.Num(s.BldgSF), st.NumComps, format.PerSF(st.MedianBldgAVPerSF), gapPhrase(st.PctAboveMedian)))
	p.body("The building part of the assessment is compared, per square foot, because the Assessor values land " +
		"separately at a neighborhood rate per square foot of land.")
	below := comps.CountBelow(*s, r.Comps)
	p.body(fmt.Sprintf("%d of the %d comparable homes have a building assessed lower per square foot than the property. "+
		"The property ranks at the %s percentile of the comparable homes by building AV per square foot.",
		below, st.NumComps, ordinal(int(math.Round(st.SubjectPercentile)))))
	if r.Search != nil {
		p.body(fmt.Sprintf("How comparable homes were chosen: %s (Assessor neighborhood %s), excluding multi-building "+
			"parcels and homes without recorded square footage or a building assessment. The %d most similar by distance "+
			"(weighted most), size, age, baths, lot, design and construction (the nearer home first when two are equally "+
			"similar) are listed on page 2.", r.Search.Description, r.Nbhd, st.NumComps))
	}

	p.heading("Uniformity figure")
	switch {
	case st.StatesReduction():
		p.body(fmt.Sprintf(
			"Applying the comparable homes' median building rate of %s per square foot to the property's %s square feet, "+
				"and adding the property's land AV of %s, gives an assessed value of %s. That is %s less than the current "+
				"assessed value of %s.",
			format.PerSF(st.MedianBldgAVPerSF), format.Num(s.BldgSF), format.Money(s.LandAV), format.Money(st.UniformityAV), format.Money(st.PotentialAVReduction), format.Money(s.AV)))
	case st.PotentialAVReduction >= 1:
		// Within 1% of the median: every surface calls this about equal, so no difference is stated.
		p.body(fmt.Sprintf(
			"Applying the comparable homes' median building rate of %s per square foot to the property's %s square feet, "+
				"and adding the property's land AV of %s, gives an assessed value of %s, about the same as the current "+
				"assessed value of %s: the building's rate is within 1%% of the median.",
			format.PerSF(st.MedianBldgAVPerSF), format.Num(s.BldgSF), format.Money(s.LandAV), format.Money(st.UniformityAV), format.Money(s.AV)))
	default:
		p.body(fmt.Sprintf(
			"Applying the comparable homes' median building rate of %s per square foot to the property's %s square feet, "+
				"and adding the property's land AV of %s, gives an assessed value of %s, which is not below the current "+
				"assessed value of %s.",
			format.PerSF(st.MedianBldgAVPerSF), format.Num(s.BldgSF), format.Money(s.LandAV), format.Money(st.UniformityAV), format.Money(s.AV)))
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

// ---------------------------------------------------------------- page 2

type column struct {
	title string
	w     float64
	right bool
	val   func(comps.Comp) string
}

// columns are the grid's, in the order PTAB decisions describe comparables.
// Titles may break onto a second line with "\n". Widths add up to the
// landscape page's 720 pt content width.
var columns = []column{
	{"PIN", 70, false, func(c comps.Comp) string { return format.PIN(c.PIN) }},
	{"Address", 70, false, func(c comps.Comp) string { return format.TitleCase(c.Address) }},
	{"Away", 32, true, func(c comps.Comp) string { return milesText(c.Miles) }},
	{"Class", 23, false, func(c comps.Comp) string { return c.Class }},
	{"Stories", 39, false, func(c comps.Comp) string { return dash(c.Features().Stories) }},
	{"Construction", 72, false, func(c comps.Comp) string { return dash(c.Features().Construction) }},
	{"Bldg\nsq ft", 29, true, func(c comps.Comp) string { return format.Num(c.BldgSF) }},
	{"Built", 24, true, func(c comps.Comp) string {
		if c.YearBuilt <= 0 {
			return "–"
		}
		return fmt.Sprint(c.YearBuilt)
	}},
	{"Beds /\nbaths", 31, true, func(c comps.Comp) string { return fmt.Sprintf("%d / %s", c.Beds, format.Baths(c.Baths())) }},
	{"Basement", 66, false, func(c comps.Comp) string { return dash(c.Features().Basement) }},
	{"Central\nair", 27, false, func(c comps.Comp) string { return dash(c.Features().Air) }},
	{"Fire-\nplaces", 25, true, func(c comps.Comp) string { return dash(c.Features().Fireplaces) }},
	{"Garage", 31, false, func(c comps.Comp) string { return dash(c.Features().Garage) }},
	{"Land\nsq ft", 31, true, func(c comps.Comp) string { return landSF(c.LandSF) }},
	{"Building\nAV", 40, true, func(c comps.Comp) string { return format.Money(c.BldgAV) }},
	{"Land\nAV", 37, true, func(c comps.Comp) string { return format.Money(c.LandAV) }},
	{"Total\nAV", 40, true, func(c comps.Comp) string { return format.Money(c.AV) }},
	{"Bldg AV\n/ sq ft", 33, true, func(c comps.Comp) string { return format.PerSF(c.BldgAVPerSF) }},
}

const (
	cellSize    = 6.8
	rowH        = 11.5
	compactRowH = 8.4 // still clears 6.8 pt type with its descenders
	cellPad     = 3.0
	gridMargin  = 36.0 // 0.5 in on the landscape grid page
)

func (p *pager) grid() {
	r := p.r
	// Up to 30 comps must fit on the one landscape page, so a long grid uses
	// shorter rows; the type size stays the same.
	p.rowH = rowH
	if len(r.Comps) > 20 {
		p.rowH = compactRowH
	}
	p.newPageSized("Comparable property grid", pageH, pageW, gridMargin) // landscape
	m, cw := p.m, p.cur.w-2*p.m
	p.para(m, cw, regular, 8, fmt.Sprintf(
		"Tax year %d assessed values from the Cook County Assessor, %s stage; building characteristics are the "+
			"Assessor's records. Homes are compared on building AV per square foot (building AV ÷ building square feet), "+
			"because the Assessor values land separately at a neighborhood rate. Evidence comparables are the most similar "+
			"homes whose building is assessed lower per square foot than the subject's. Away = straight-line distance "+
			"from the subject.", r.Year, r.AVStage))

	tableW := 0.0
	for _, c := range columns {
		tableW += c.w
	}
	x0 := m + (cw-tableW)/2

	top := p.y
	p.header(x0)
	p.section(x0, tableW, "Subject property")
	p.row(x0, comps.Comp{Home: *r.Subject, Miles: -1}, bold, 0.92)
	ev := map[string]bool{}
	p.section(x0, tableW, fmt.Sprintf("Evidence comparables (%d): similar homes with the building assessed lower per sq ft", len(r.Evidence)))
	for _, c := range r.Evidence {
		ev[c.PIN] = true
		p.row(x0, c, regular, -1)
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
			p.row(x0, c, regular, -1)
		}
	}
	p.strokeRect(x0, top, tableW, p.y-top, 0.8)

	p.y += 10
	st := r.Stats
	// The subject's rate unrounded, as page 1 and the grid's subject row print
	// it: st.SubjectBldgAVPerSF is rounded to the cent first, so at a half cent
	// it printed one cent more than those two.
	p.para(m, cw, bold, 8, fmt.Sprintf("Median building AV / sq ft of the %d comparable homes: $%.2f.  Subject: $%.2f (%s the median).",
		st.NumComps, st.MedianBldgAVPerSF, r.Subject.BldgAVPerSF, gapPhrase(st.PctAboveMedian)))
	if len(r.Evidence) > 0 {
		p.y -= 3
		p.text(m, p.y, bold, 7.5, "Differences of each evidence comparable from the subject (theirs vs the subject's):")
		p.y += 9
		for i, c := range r.Evidence {
			addr := format.TitleCase(c.Address)
			if addr == "" {
				addr = "PIN " + format.PIN(c.PIN)
			}
			p.para(m, cw, regular, 7, fmt.Sprintf("%d. %s: %s", i+1, addr, c.Differences))
			p.y -= 4
			if p.rowH < rowH {
				p.y-- // a long grid leaves less room below it
			}
		}
	}
}

// header draws the column titles, each on one or two lines.
func (p *pager) header(x0 float64) {
	const h = rowH + 7
	x := x0
	for i, c := range columns {
		p.fillRect(x, p.y, c.w, h, 0.82)
		if i > 0 {
			p.line(x, p.y, x, p.y+h, 0.4)
		}
		lines := strings.Split(c.title, "\n")
		base := p.y + h - 4.5 - float64(len(lines)-1)*(cellSize+1)
		for j, ln := range lines {
			y := base + float64(j)*(cellSize+1)
			if c.right {
				p.textRight(x+c.w-cellPad, y, bold, cellSize, ln)
			} else {
				p.text(x+cellPad, y, bold, cellSize, ln)
			}
		}
		x += c.w
	}
	p.y += h
	p.line(x0, p.y, x, p.y, 0.4)
}

func (p *pager) row(x0 float64, c comps.Comp, f font, gray float64) {
	p.cells(x0, p.rowH, f, gray, func(col column) string { return col.val(c) })
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
		p.cell(x, p.y, c.w, h, f, val(c), c.right, c.title == "Address")
		x += c.w
	}
	p.y += h
	p.line(x0, p.y, x, p.y, 0.4)
}

// section draws a full-width label row.
func (p *pager) section(x0, w float64, title string) {
	p.fillRect(x0, p.y, w, p.rowH, 0.96)
	p.text(x0+cellPad, p.y+baseline(p.rowH), bold, cellSize, title)
	p.y += p.rowH
	p.line(x0, p.y, x0+w, p.y, 0.4)
}

// cell writes s in a cell. Only a column marked truncate (the address) is
// cut to fit; any other value is drawn smaller instead, so a figure is never
// shortened into a different number.
func (p *pager) cell(x, y, w, h float64, f font, s string, right, truncate bool) {
	s, size := cellText(f, w-2*cellPad, s, truncate)
	base := y + baseline(h)
	if right {
		p.textRight(x+w-cellPad, base, f, size, s)
	} else {
		p.text(x+cellPad, base, f, size, s)
	}
}

// baseline is where text sits in a row of height h: 4.5 pt above the bottom
// in a full row. A compact row centers the type between its cap height and
// descenders (Helvetica: 0.718 and 0.207 of the size), so caps don't touch
// the rule above.
func baseline(h float64) float64 {
	if h >= rowH {
		return h - 4.5
	}
	return (h + (0.718-0.207)*cellSize) / 2
}

// cellText returns s as drawn in a cell of inner width w, and its font size.
func cellText(f font, w float64, s string, truncate bool) (string, float64) {
	if truncate {
		return fit(f, cellSize, w, s), cellSize
	}
	size := cellSize
	if tw := textWidth(f, cellSize, s); tw > w {
		size = cellSize * w / tw
	}
	return s, size
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
		"Choose \"" + bor.AppealType + "\" as the type of appeal and enter PIN " + format.PIN(r.PIN) + ". When asked if " +
			"you are a Board of Review attorney, answer No. Choose your appellant type (owner).",
		"Decide whether you want a hearing. It is optional, held by phone for homeowners, and the Board says you are " +
			"not penalized for skipping it. You choose this when you file.",
		"Enter comparable properties if you like (optional: the Board's analysts also pull their own). The evidence " +
			"comparables on page 2 are similar homes in your Assessor neighborhood whose building is assessed lower per " +
			"square foot than yours; the other homes listed there are not.",
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
	day := format.LongDate
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
// Figures use internal/format, as the web page and the CLI do. What is left
// here is the PDF's own wording and placeholders.

func gapPhrase(g float64) string { return format.Gap(g, comps.InLine(g)) }

// milesText is the distance from the subject; "–" when unknown (and for the
// subject itself).
func milesText(mi float64) string { return format.Miles(mi, "–") }

// dash shows a characteristic that isn't on record as "–".
func dash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}

func landSF(v float64) string {
	if v <= 0 {
		return "–"
	}
	return format.Num(v)
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
