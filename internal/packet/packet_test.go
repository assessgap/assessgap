package packet

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/fixture"
)

func TestMain(m *testing.M) { os.Exit(fixture.Main(m)) }

// when is Nov 24, 2026 in Chicago (midnight UTC would still be Nov 23 there).
var when = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

// TestRenderEveryHome renders the evidence report for every supported home in
// the fixture: each must be a complete 3-page PDF whose content never runs
// into the footer (build returns an error if it does), with every comparable
// home in the grid exactly once. The text of three reports is in the goldens.
func TestRenderEveryHome(t *testing.T) {
	db := fixture.Open(t)
	eng := comps.New(db)
	eng.CachePools = true
	rendered := 0
	for _, pin := range supportedPINs(t, db) {
		r, err := eng.Report(context.Background(), pin, 0)
		if err != nil {
			t.Fatalf("%s: %v", pin, err)
		}
		var buf bytes.Buffer
		if err := Render(&buf, r, when); err != nil {
			t.Errorf("%s: %v", pin, err)
			continue
		}
		rendered++
		out := buf.Bytes()
		if !bytes.HasPrefix(out, []byte("%PDF-1.4")) || !bytes.HasSuffix(out, []byte("%%EOF\n")) {
			t.Errorf("%s: not a complete PDF", pin)
		}
		if got := countPages(out); got != Pages {
			t.Errorf("%s: %d page objects, want %d", pin, got, Pages)
		}
		txt, err := Text(r, when)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range r.Comps {
			if n := strings.Count(txt, formatPIN(c.PIN)); n != 1 {
				t.Errorf("%s: comp %s appears %d times", pin, c.PIN, n)
			}
		}
		// The grid truncates a value that doesn't fit its cell. Only an
		// address may be shortened; a cut figure would be a wrong number.
		rows := append([]comps.Comp{{Home: *r.Subject, Miles: -1}}, r.Comps...)
		for i, c := range rows {
			f := regular
			if i == 0 {
				f = bold
			}
			for _, col := range columns {
				if v := col.val(c); col.title != "Address" && textWidth(f, cellSize, v) > col.w-2*cellPad {
					t.Errorf("%s: %s column cuts %q", pin, col.title, v)
				}
			}
		}
	}
	// The frozen fixture has 2,102 supported homes; this keeps the loop honest.
	if rendered != 2102 {
		t.Errorf("rendered %d reports, want 2102", rendered)
	}
}

// TestGridFitsEveryValue covers what the fixture may not: every value the
// Assessor publishes for the decoded columns (docs/data-inventory.md), in
// bold (the subject row), and the widest figures, must fit without cutting.
func TestGridFitsEveryValue(t *testing.T) {
	var homes []comps.Home
	for _, typ := range []string{"1 Story", "1.5 Story", "2 Story", "3 Story +", "Split Level"} {
		for _, wall := range []string{"Frame", "Masonry", "Frame + Masonry", "Stucco"} {
			for _, b := range []string{"Full", "Partial", "Slab", "Crawl"} {
				for _, fin := range []string{"Formal Rec Room", "Unfinished", "Apartment"} {
					homes = append(homes, comps.Home{TypeResd: typ, ExtWall: wall, Basement: b, BsmtFin: fin})
				}
			}
		}
	}
	for _, g := range []string{"0 cars", "1 cars", "1.5 cars", "2 cars", "2.5 cars", "3 cars", "3.5 cars", "4 cars"} {
		for _, air := range []string{"Central A/C", "No Central A/C"} {
			homes = append(homes, comps.Home{Garage: g, Air: air})
		}
	}
	homes = append(homes, comps.Home{}) // nothing on record
	for _, h := range homes {
		c := comps.Comp{Home: h, Miles: 0.05}
		for _, col := range columns {
			if v := col.val(c); col.title != "Address" && textWidth(bold, cellSize, v) > col.w-2*cellPad {
				t.Errorf("%s column cuts %q", col.title, v)
			}
		}
	}
	// Beyond the largest values in the county's 2026 records (AV $5.5M, 87,865
	// building sq ft, 8.8M land sq ft, 17 beds, 42 full and 71 half baths, 22
	// fireplaces): drawn whole, smaller if need be, but still legible.
	huge := comps.Comp{Miles: 88.8, Home: comps.Home{PIN: "88888888888888", BldgSF: 99999, LandSF: 9999999,
		YearBuilt: 2026, Beds: 18, FullBaths: 88, HalfBaths: 88, Fireplaces: 88,
		AV: 9999999, BldgAV: 9999999, LandAV: 9999999, BldgAVPerSF: 8888.88}}
	for _, col := range columns {
		v := col.val(huge)
		if col.title == "Address" {
			continue
		}
		if got, size := cellText(bold, col.w-2*cellPad, v, false); got != v || size < 5 {
			t.Errorf("%s column draws %q at %.1f pt, want %q at 5 pt or more", col.title, got, size, v)
		}
	}
}

// supportedPINs lists the fixture's homes that get a comparison.
func supportedPINs(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT a.pin FROM assessment a JOIN res_char r USING (pin)
		WHERE a.year = 2026 AND r.num_cards = 1 AND r.bldg_sf > 0
		  AND a.class IN ('202','203','204','205','206','207','208','209','210','234','278','295')
		ORDER BY a.pin`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var pins []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		pins = append(pins, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return pins
}

func countPages(pdf []byte) int {
	return len(regexp.MustCompile(`/Type /Page /`).FindAll(pdf, -1))
}

func TestUnsupported(t *testing.T) {
	condo, err := comps.New(fixture.Open(t)).Report(context.Background(), "13331120411001", 0)
	if err != nil {
		t.Fatal(err)
	}
	s := comps.Home{PIN: "S"}
	tests := []struct {
		name string
		r    *comps.Report
	}{
		{"no report", nil},
		{"condo", condo},
		{"subject but no statistics", &comps.Report{PIN: "S", Subject: &s}},
	}
	for _, tt := range tests {
		if err := Render(&bytes.Buffer{}, tt.r, when); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", tt.name, err)
		}
	}
}

func TestPDFString(t *testing.T) {
	tests := []struct{ in, want string }{
		{`a (b) \c`, `a \(b\) \\c`},     // escaping
		{"1–2 • ÷", "1\x962 \x95 \xf7"}, // WinAnsi map, then Latin-1
		{"≥5%", ">=5%"},                 // ASCII fallback
		{"日本", "??"},                    // unknown
	}
	for _, tt := range tests {
		if got := pdfString(tt.in); got != tt.want {
			t.Errorf("pdfString(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTextWidth(t *testing.T) {
	tests := []struct {
		f    font
		s    string
		want float64
	}{
		{regular, "Hello", 22.78}, // Helvetica 722+556+222+222+556 = 2278 at 10 pt
		{bold, "Hello", 24.45},    // Helvetica-Bold 722+556+278+278+611 = 2445
		{regular, "•", 3.5},       // a WinAnsi character outside ASCII (350)
	}
	for _, tt := range tests {
		if got := textWidth(tt.f, 10, tt.s); got < tt.want-0.001 || got > tt.want+0.001 {
			t.Errorf("textWidth(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestWrap(t *testing.T) {
	// At 10 pt, "Hello Hello" is 22.78 + 2.78 + 22.78 = 48.34 wide.
	tests := []struct {
		name string
		maxW float64
		s    string
		want []string
	}{
		{"two fit", 48.4, "Hello Hello Hello", []string{"Hello Hello", "Hello"}},
		{"just too narrow for two", 48.3, "Hello Hello Hello", []string{"Hello", "Hello", "Hello"}},
		{"newlines kept, blank line too", 100, "Hello\n\nHello", []string{"Hello", "", "Hello"}},
	}
	for _, tt := range tests {
		if got := wrap(regular, 10, tt.maxW, tt.s); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: wrap = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestOrdinal(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{1, "1st"}, {2, "2nd"}, {3, "3rd"}, {11, "11th"}, {13, "13th"}, {21, "21st"}, {100, "100th"},
	}
	for _, tt := range tests {
		if got := ordinal(tt.n); got != tt.want {
			t.Errorf("ordinal(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
