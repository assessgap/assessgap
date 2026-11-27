package packet

import (
	"fmt"
	"strings"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/format"
)

// Page 2: the comparable-property grid, on a landscape page.

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
