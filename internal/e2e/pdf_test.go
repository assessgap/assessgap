package e2e

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
)

var (
	startxrefRe = regexp.MustCompile(`startxref\s+(\d+)\s+%%EOF\s*$`)
	xrefEntryRe = regexp.MustCompile(`^(\d{10}) (\d{5}) ([nf])\s*$`)
	pagesRe     = regexp.MustCompile(`/Type /Pages\b[^>]*/Count (\d+)`)
)

// parsePDF checks the structure a PDF reader relies on: the header, the
// trailer's startxref pointing at the cross-reference table, and every
// in-use entry pointing at its "N 0 obj". It returns the page count from the
// page tree. A truncated or half-written PDF fails here.
func parsePDF(b []byte) (pages int, err error) {
	if !bytes.HasPrefix(b, []byte("%PDF-1.")) {
		return 0, fmt.Errorf("no %%PDF-1.x header")
	}
	m := startxrefRe.FindSubmatch(b)
	if m == nil {
		return 0, fmt.Errorf("no startxref/%%%%EOF trailer")
	}
	off, _ := strconv.Atoi(string(m[1]))
	if off >= len(b) || !bytes.HasPrefix(b[off:], []byte("xref\n")) {
		return 0, fmt.Errorf("startxref %d does not point at the xref table", off)
	}
	lines := bytes.Split(b[off:], []byte("\n"))
	var first, count int
	if _, err := fmt.Sscanf(string(lines[1]), "%d %d", &first, &count); err != nil {
		return 0, fmt.Errorf("xref subsection header %q: %w", lines[1], err)
	}
	if count < 2 || len(lines) < 2+count {
		return 0, fmt.Errorf("xref has %d entries", count)
	}
	for i := 0; i < count; i++ {
		e := xrefEntryRe.FindSubmatch(lines[2+i])
		if e == nil {
			return 0, fmt.Errorf("xref entry %d: %q", i, lines[2+i])
		}
		if string(e[3]) == "f" {
			continue
		}
		o, _ := strconv.Atoi(string(e[1]))
		want := fmt.Sprintf("%d 0 obj", first+i)
		if o >= len(b) || !bytes.HasPrefix(b[o:], []byte(want)) {
			return 0, fmt.Errorf("xref entry %d points at offset %d, not %q", i, o, want)
		}
	}
	p := pagesRe.FindSubmatch(b)
	if p == nil {
		return 0, fmt.Errorf("no page tree")
	}
	return strconv.Atoi(string(p[1]))
}
