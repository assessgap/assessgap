package packet

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/store"
)

const sampleDB = "../../data-sample/cook.db"

func openSample(t *testing.T) *sql.DB {
	t.Helper()
	if _, err := os.Stat(sampleDB); err != nil {
		t.Skipf("sample DB missing (%v)", err)
	}
	db, err := store.OpenReadOnly(sampleDB)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

var when = time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)

func TestRenderSample(t *testing.T) {
	db := openSample(t)
	eng := comps.New(db)
	// A "possible", a "weak" and the first few supported PINs in the sample.
	pins := []string{"13281000290000", "13281000270000"}
	rows, err := db.Query(`SELECT a.pin FROM assessment a JOIN res_char r USING (pin)
		WHERE a.year = 2026 AND r.num_cards = 1 AND r.bldg_sf > 0 AND a.class IN ('202','203','205','211','234','278','295')
		ORDER BY a.pin LIMIT 25`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var p string
		rows.Scan(&p)
		pins = append(pins, p)
	}
	rows.Close()

	rendered := 0
	for _, pin := range pins {
		r, err := eng.Report(context.Background(), pin, 0)
		if err != nil {
			t.Fatalf("%s: %v", pin, err)
		}
		var buf bytes.Buffer
		err = Render(&buf, r, when)
		if r.Unsupported != "" {
			if !errors.Is(err, ErrUnsupported) {
				t.Errorf("%s unsupported (%s): err = %v, want ErrUnsupported", pin, r.Unsupported, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", pin, err)
		}
		rendered++
		out := buf.Bytes()
		if !bytes.HasPrefix(out, []byte("%PDF-1.4")) || !bytes.HasSuffix(out, []byte("%%EOF\n")) {
			t.Errorf("%s: not a complete PDF", pin)
		}
		if got := countPages(out); got != Pages {
			t.Errorf("%s: %d page objects, want %d", pin, got, Pages)
		}
		if !bytes.Contains(out, []byte("/Count 3 ")) {
			t.Errorf("%s: page tree /Count is not 3", pin)
		}

		txt, err := Text(r, when)
		if err != nil {
			t.Fatal(err)
		}
		checkText(t, pin, r, txt)
	}
	if rendered < 2 {
		t.Fatalf("rendered only %d packets", rendered)
	}
}

func countPages(pdf []byte) int {
	return len(regexp.MustCompile(`/Type /Page /`).FindAll(pdf, -1))
}

func checkText(t *testing.T, pin string, r *comps.Report, txt string) {
	t.Helper()
	for _, want := range []string{formatPIN(r.PIN), NotAffiliated, "Comparable property grid", "Filing steps", "Property Over-Assessed", "Page 3 of 3"} {
		if !strings.Contains(txt, want) {
			t.Errorf("%s: packet text missing %q", pin, want)
		}
	}
	// Every comp appears in the grid exactly once (subject once in the grid).
	for _, c := range r.Comps {
		if n := strings.Count(txt, formatPIN(c.PIN)); n != 1 {
			t.Errorf("%s: comp %s appears %d times", pin, c.PIN, n)
		}
	}
	// Appraisal-law guardrail: the disclaimer may say "not an appraisal"; nothing else may.
	body := strings.ToLower(strings.ReplaceAll(txt, "\n", " "))
	body = strings.ReplaceAll(body, strings.ToLower(r.Disclaimer), "")
	for _, banned := range []string{"market value", "apprais", "worth", "you should", "we recommend"} {
		if strings.Contains(body, banned) {
			t.Errorf("%s: packet text contains %q", pin, banned)
		}
	}
}

func TestUnsupported(t *testing.T) {
	r := &comps.Report{PIN: "13281000000000", Unsupported: "Condominium units (class 299) are not supported yet."}
	if err := Render(&bytes.Buffer{}, r, when); !errors.Is(err, ErrUnsupported) {
		t.Errorf("err = %v, want ErrUnsupported", err)
	}
}

func TestPDFString(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", "plain"},
		{`a (b) \c`, `a \(b\) \\c`},
		{"1–2 · ×", "1\x962 \xb7 \xd7"},
		{"≥5%", ">=5%"},
		{"日本", "??"},
	}
	for _, tt := range tests {
		if got := pdfString(tt.in); got != tt.want {
			t.Errorf("pdfString(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWrap(t *testing.T) {
	s := "The quick brown fox jumps over the lazy dog and keeps running far away"
	lines := wrap(regular, 10, 120, s)
	if len(lines) < 3 {
		t.Fatalf("expected wrapping, got %q", lines)
	}
	for _, ln := range lines {
		if w := textWidth(regular, 10, ln); w > 120 {
			t.Errorf("line %q is %.1f wide, max 120", ln, w)
		}
	}
	if strings.Join(lines, " ") != s {
		t.Errorf("wrap lost words: %q", lines)
	}
	// Helvetica: "Hello" = 722+556+222+222+556 = 2278 → 22.78 at 10 pt.
	if w := textWidth(regular, 10, "Hello"); w < 22.77 || w > 22.79 {
		t.Errorf("textWidth(Hello) = %v", w)
	}
}

func TestOrdinal(t *testing.T) {
	for n, want := range map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 13: "13th", 21: "21st", 100: "100th", 0: "0th"} {
		if got := ordinal(n); got != want {
			t.Errorf("ordinal(%d) = %q, want %q", n, got, want)
		}
	}
}
