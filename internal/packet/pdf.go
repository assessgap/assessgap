package packet

// A deliberately small PDF writer: letter pages, the two standard Helvetica
// fonts (no embedding needed), text, lines and filled rectangles. That is all
// the packet needs, and it keeps the module free of new dependencies.
// Coordinates are points from the top-left corner of the page.

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"strings"
)

const (
	pageW = 612.0 // US letter, points
	pageH = 792.0
)

type font int

const (
	regular font = iota
	bold
)

var fontNames = [...]string{regular: "Helvetica", bold: "Helvetica-Bold"}

type doc struct {
	title string
	pages []*page
	cur   *page
	texts []string // every string drawn, in order; for tests and text checks
}

// page is one page's content stream and size in points.
type page struct {
	bytes.Buffer
	w, h float64
}

func newDoc(title string) *doc { return &doc{title: title} }

// addPage starts a page of the given size: pageW × pageH is portrait letter,
// pageH × pageW landscape.
func (d *doc) addPage(w, h float64) {
	d.cur = &page{w: w, h: h}
	d.pages = append(d.pages, d.cur)
}

// text draws s with its baseline at y.
func (d *doc) text(x, y float64, f font, size float64, s string) {
	d.texts = append(d.texts, s)
	fmt.Fprintf(d.cur, "BT /F%d %.2f Tf %.2f %.2f Td (%s) Tj ET\n", f+1, size, x, d.cur.h-y, pdfString(s))
}

// textRight draws s so that it ends at x.
func (d *doc) textRight(x, y float64, f font, size float64, s string) {
	d.text(x-textWidth(f, size, s), y, f, size, s)
}

func (d *doc) line(x1, y1, x2, y2, width float64) {
	fmt.Fprintf(d.cur, "%.2f w %.2f %.2f m %.2f %.2f l S\n", width, x1, d.cur.h-y1, x2, d.cur.h-y2)
}

// fillRect fills a rectangle with a gray level (0 black … 1 white).
func (d *doc) fillRect(x, y, w, h, gray float64) {
	fmt.Fprintf(d.cur, "%.3f g %.2f %.2f %.2f %.2f re f 0 g\n", gray, x, d.cur.h-y-h, w, h)
}

func (d *doc) strokeRect(x, y, w, h, width float64) {
	fmt.Fprintf(d.cur, "%.2f w %.2f %.2f %.2f %.2f re S\n", width, x, d.cur.h-y-h, w, h)
}

// writeTo serializes the document.
func (d *doc) writeTo(w io.Writer) error {
	var out bytes.Buffer
	var offsets []int
	obj := func(body string) {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")

	// 1 catalog, 2 pages, 3–4 fonts, 5 info, then (page, content) pairs.
	const firstPage = 6
	kids := make([]string, len(d.pages))
	for i := range d.pages {
		kids[i] = fmt.Sprintf("%d 0 R", firstPage+2*i)
	}
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(d.pages)))
	for _, name := range fontNames {
		obj(fmt.Sprintf("<< /Type /Font /Subtype /Type1 /BaseFont /%s /Encoding /WinAnsiEncoding >>", name))
	}
	obj(fmt.Sprintf("<< /Title (%s) /Producer (AssessGap) >>", pdfString(d.title)))
	for i, p := range d.pages {
		var z bytes.Buffer
		zw := zlib.NewWriter(&z)
		if _, err := zw.Write(p.Bytes()); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] "+
			"/Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>",
			p.w, p.h, firstPage+2*i+1))
		obj(fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream", z.Len(), z.Bytes()))
	}

	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, off := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R /Info 5 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	_, err := w.Write(out.Bytes())
	return err
}

// winAnsi maps the few non-ASCII characters we use to WinAnsiEncoding bytes.
var winAnsi = map[rune]byte{
	'–': 0x96, '—': 0x97, '•': 0x95, '‘': 0x91, '’': 0x92, '“': 0x93, '”': 0x94,
	'·': 0xB7, '×': 0xD7, '§': 0xA7,
}

// ascii replaces characters Helvetica/WinAnsi can't show.
var asciiFallback = strings.NewReplacer("≥", ">=", "≤", "<=", "→", "->", "…", "...")

// encode converts s to WinAnsi bytes; unknown characters become '?'.
func encode(s string) []byte {
	s = asciiFallback.Replace(s)
	b := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 32 && r < 127:
			b = append(b, byte(r))
		case winAnsi[r] != 0:
			b = append(b, winAnsi[r])
		case r >= 0xA0 && r <= 0xFF: // Latin-1 range matches WinAnsi
			b = append(b, byte(r))
		default:
			b = append(b, '?')
		}
	}
	return b
}

// pdfString encodes and escapes s for a literal (...) string.
func pdfString(s string) string {
	var b strings.Builder
	for _, c := range encode(s) {
		switch c {
		case '\\', '(', ')':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// Advance widths (1/1000 em) for characters 32–126, from the Adobe AFM files.
var widths = [...][95]int{
	regular: {
		278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278,
		556, 556, 556, 556, 556, 556, 556, 556, 556, 556,
		278, 278, 584, 584, 584, 556, 1015,
		667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833,
		722, 778, 667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611,
		278, 278, 278, 469, 556, 333,
		556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500, 222, 833,
		556, 556, 556, 556, 333, 500, 278, 556, 500, 722, 500, 500, 500,
		334, 260, 334, 584,
	},
	bold: {
		278, 333, 474, 556, 556, 889, 722, 238, 333, 333, 389, 584, 278, 333, 278, 278,
		556, 556, 556, 556, 556, 556, 556, 556, 556, 556,
		333, 333, 584, 584, 584, 611, 975,
		722, 722, 722, 722, 667, 611, 778, 722, 278, 556, 722, 611, 833,
		722, 778, 667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611,
		333, 278, 333, 584, 556, 333,
		556, 611, 556, 611, 556, 333, 611, 611, 278, 278, 556, 278, 889,
		611, 611, 611, 611, 389, 556, 333, 611, 556, 778, 556, 556, 500,
		389, 280, 389, 584,
	},
}

var extraWidths = map[byte]int{0x96: 556, 0x97: 1000, 0x95: 350, 0x91: 222, 0x92: 222,
	0x93: 333, 0x94: 333, 0xB7: 278, 0xD7: 584, 0xA7: 556, 0xF7: 584}

func textWidth(f font, size float64, s string) float64 {
	total := 0
	for _, c := range encode(s) {
		if c >= 32 && c < 127 {
			total += widths[f][c-32]
		} else if w, ok := extraWidths[c]; ok {
			total += w
		} else {
			total += 556
		}
	}
	return float64(total) * size / 1000
}

// wrap breaks s into lines no wider than maxW. Explicit newlines are kept.
func wrap(f font, size, maxW float64, s string) []string {
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		cur := words[0]
		for _, w := range words[1:] {
			if textWidth(f, size, cur+" "+w) <= maxW {
				cur += " " + w
			} else {
				lines = append(lines, cur)
				cur = w
			}
		}
		lines = append(lines, cur)
	}
	return lines
}
