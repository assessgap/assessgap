package web

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/assessgap/assessgap/internal/fixture"
	"github.com/assessgap/assessgap/internal/store"
)

func TestMain(m *testing.M) { os.Exit(fixture.Main(m)) }

// newTestServer serves the frozen fixture database (testdata/fixture).
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cook := fixture.Open(t)
	app, err := store.OpenApp(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	site, err := New(cook, app, true, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(site.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestReportPDF(t *testing.T) {
	ts := newTestServer(t)
	tests := []struct {
		name   string
		path   string
		status []int // any of these
		pdf    bool
	}{
		{"supported", "/check/13331030130000/report.pdf", []int{200}, true},
		{"supported with dashes", "/check/13-33-103-013-0000/report.pdf", []int{200}, true},
		{"weak verdict still renders", "/check/13331020180000/report.pdf", []int{200}, true},
		{"condo is unsupported", "/check/13331120411001/report.pdf", []int{404}, false},
		{"unknown PIN", "/check/99999999999999/report.pdf", []int{404}, false},
		{"malformed PIN", "/check/1328abc/report.pdf", []int{400, 404}, false},
		{"too short", "/check/1328/report.pdf", []int{400, 404}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get(ts.URL + tt.path)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			ok := false
			for _, s := range tt.status {
				ok = ok || resp.StatusCode == s
			}
			if !ok {
				t.Fatalf("status %d, want %v", resp.StatusCode, tt.status)
			}
			isPDF := bytes.HasPrefix(body, []byte("%PDF-"))
			if isPDF != tt.pdf {
				t.Fatalf("PDF magic = %v, want %v", isPDF, tt.pdf)
			}
			if !tt.pdf {
				return
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/pdf" {
				t.Errorf("Content-Type %q", ct)
			}
			cd := resp.Header.Get("Content-Disposition")
			pin := strings.TrimSuffix(strings.TrimPrefix(tt.path, "/check/"), "/report.pdf")
			pin = strings.ReplaceAll(pin, "-", "")
			if !strings.HasPrefix(cd, "attachment;") || !strings.Contains(cd, `filename="assessgap-`+pin+`.pdf"`) {
				t.Errorf("Content-Disposition %q", cd)
			}
		})
	}
}

// TestReportPageOffersPDF checks the "Your options" item and the absence of
// any price on the report page.
func TestReportPageOffersPDF(t *testing.T) {
	ts := newTestServer(t)
	tests := []struct {
		pin   string
		offer bool
	}{
		{"13331030130000", true},  // strong
		{"16171210300000", true},  // possible
		{"13331020180000", false}, // weak
	}
	for _, tt := range tests {
		resp, err := http.Get(ts.URL + "/check/" + tt.pin)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		page := string(body)
		if resp.StatusCode != 200 {
			t.Fatalf("%s: status %d", tt.pin, resp.StatusCode)
		}
		if got := strings.Contains(page, "/check/"+tt.pin+"/report.pdf"); got != tt.offer {
			t.Errorf("%s: PDF link shown = %v, want %v", tt.pin, got, tt.offer)
		}
		// ("$29" alone would match AV-per-sq-ft figures such as "$29.40".)
		// "upfront fee" is allowed: the attorney option describes attorneys' fees (restored in f9046db).
		for _, banned := range []string{"$29:", "Coming soon", "paid us"} {
			if strings.Contains(page, banned) {
				t.Errorf("%s: page contains %q", tt.pin, banned)
			}
		}
		if !strings.Contains(page, "https://github.com/assessgap/assessgap") {
			t.Errorf("%s: footer lacks the source link", tt.pin)
		}
	}
}
