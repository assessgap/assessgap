// Package packet renders a printable filing packet (PDF) from a comps report:
// a summary page, a comparable-property grid, and a filing checklist.
//
// Like the rest of the product, it states facts about assessed values only.
// It never states or estimates market value and never tells the homeowner
// what to argue.
//
// This file holds the entry points and the layout primitives. Each page has
// its own file: summary.go (1), grid.go (2), steps.go (3). words.go has the
// PDF's own wording and placeholders, and pdf.go the minimal PDF writer.
package packet

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

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
