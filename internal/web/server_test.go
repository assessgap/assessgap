package web

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/assessgap/assessgap/internal/fixture"
	"github.com/assessgap/assessgap/internal/store"
)

func TestMain(m *testing.M) { os.Exit(fixture.Main(m)) }

// newSite serves the frozen fixture database (testdata/fixture) with an
// empty app database.
func newSite(t *testing.T, dev bool) (*Server, *httptest.Server) {
	t.Helper()
	app, err := store.OpenApp(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	site, err := New(fixture.Open(t), app, dev, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(site.Handler())
	t.Cleanup(ts.Close)
	return site, ts
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	_, ts := newSite(t, true)
	return ts
}

// get fetches url and returns the status, headers and body.
func get(t *testing.T, url string) (int, http.Header, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Header, body
}

func TestReportPDF(t *testing.T) {
	ts := newTestServer(t)
	tests := []struct {
		name        string
		path        string
		status      int
		disposition string // "" for errors, which must not be PDFs
	}{
		{"supported", "/check/13331030130000/report.pdf", 200, `attachment; filename="assessgap-13331030130000.pdf"`},
		{"dashes are stripped", "/check/13-33-103-013-0000/report.pdf", 200, `attachment; filename="assessgap-13331030130000.pdf"`},
		{"unsupported property", "/check/13331120411001/report.pdf", 404, ""},
		{"unknown PIN", "/check/99999999999999/report.pdf", 404, ""},
		{"malformed PIN", "/check/1328abc/report.pdf", 404, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, header, body := get(t, ts.URL+tt.path)
			if status != tt.status {
				t.Fatalf("status %d, want %d", status, tt.status)
			}
			if got := header.Get("Content-Disposition"); got != tt.disposition {
				t.Errorf("Content-Disposition %q, want %q", got, tt.disposition)
			}
			if isPDF := bytes.HasPrefix(body, []byte("%PDF-")); isPDF != (tt.disposition != "") {
				t.Errorf("PDF body = %v, want %v", isPDF, tt.disposition != "")
			}
		})
	}
}

// TestReportPageOffersPDF checks that the evidence report is offered only
// when the report lists lower-assessed homes (any tier but the lowest).
func TestReportPageOffersPDF(t *testing.T) {
	ts := newTestServer(t)
	tests := []struct {
		name, pin string
		offer     bool
	}{
		{"lower-assessed homes to list", "13331030130000", true},
		{"lowest tier", "13331020180000", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, body := get(t, ts.URL+"/check/"+tt.pin)
			if status != 200 {
				t.Fatalf("status %d", status)
			}
			if got := bytes.Contains(body, []byte("/check/"+tt.pin+"/report.pdf")); got != tt.offer {
				t.Errorf("PDF link shown = %v, want %v", got, tt.offer)
			}
		})
	}
}

// TestSecurityHeaders checks the headers every response carries, and that
// only a dev server asks search engines not to index it.
func TestSecurityHeaders(t *testing.T) {
	for _, dev := range []bool{false, true} {
		_, ts := newSite(t, dev)
		for _, path := range []string{"/", "/check/13331030130000", "/nope"} {
			_, h, _ := get(t, ts.URL+path)
			want := map[string]string{
				"Content-Security-Policy": "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; " +
					"form-action 'self'; frame-ancestors 'none'; base-uri 'self'",
				"X-Content-Type-Options": "nosniff",
				"Referrer-Policy":        "strict-origin-when-cross-origin",
			}
			if dev {
				want["X-Robots-Tag"] = "noindex, nofollow"
			}
			for k, v := range want {
				if h.Get(k) != v {
					t.Errorf("dev=%v %s: %s = %q, want %q", dev, path, k, h.Get(k), v)
				}
			}
			if !dev && h.Get("X-Robots-Tag") != "" {
				t.Errorf("production %s: X-Robots-Tag %q", path, h.Get("X-Robots-Tag"))
			}
		}
	}
}

func postNotify(t *testing.T, ts *httptest.Server, form url.Values, header map[string]string) (int, string) {
	t.Helper()
	req, err := http.NewRequest("POST", ts.URL+"/notify", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(body) + "\nRetry-After: " + resp.Header.Get("Retry-After")
}

// TestNotifyCrossOrigin checks that another site can't post the signup form.
func TestNotifyCrossOrigin(t *testing.T) {
	_, ts := newSite(t, false)
	form := url.Values{"email": {"a@example.com"}}
	if code, _ := postNotify(t, ts, form, map[string]string{"Sec-Fetch-Site": "cross-site"}); code != http.StatusForbidden {
		t.Errorf("cross-site post: status %d, want 403", code)
	}
	if code, _ := postNotify(t, ts, form, map[string]string{"Sec-Fetch-Site": "same-origin"}); code != 200 {
		t.Errorf("same-origin post: status %d, want 200", code)
	}
}

// TestNotifyRateLimit checks 10 signups per client per 10 minutes. The test
// client is loopback, so X-Forwarded-For (set by Caddy in production) names it.
func TestNotifyRateLimit(t *testing.T) {
	_, ts := newSite(t, false)
	form := url.Values{"email": {"a@example.com"}}
	for i := 1; i <= 10; i++ {
		if code, _ := postNotify(t, ts, form, map[string]string{"X-Forwarded-For": "203.0.113.1"}); code != 200 {
			t.Fatalf("request %d: status %d, want 200", i, code)
		}
	}
	code, body := postNotify(t, ts, form, map[string]string{"X-Forwarded-For": "203.0.113.1"})
	if code != http.StatusTooManyRequests || !strings.HasSuffix(body, "Retry-After: 600") {
		t.Errorf("11th request: status %d, %q; want 429 with Retry-After 600", code, body)
	}
	if code, _ := postNotify(t, ts, form, map[string]string{"X-Forwarded-For": "203.0.113.2"}); code != 200 {
		t.Errorf("another client: status %d, want 200", code)
	}
}

// TestNotifyInput checks what the signup form stores.
func TestNotifyInput(t *testing.T) {
	site, ts := newSite(t, false)
	tests := []struct {
		name   string
		form   url.Values
		status int
		stored string // "email|pin|township|reason", or "" for nothing stored
	}{
		{"valid, with PIN", url.Values{"email": {"Ann@Example.com"}, "pin": {"13331030130000"}, "reason": {"unsupported"}},
			200, "ann@example.com|13331030130000|Jefferson|unsupported"},
		{"bad PIN and reason are dropped", url.Values{"email": {"bo@example.com"}, "pin": {"1333'; --"}, "reason": {"<script>"}},
			200, "bo@example.com|||township-opens"},
		{"township page", url.Values{"email": {"di@example.com"}, "township": {"lake-view"}, "reason": {"township-opens"}},
			200, "di@example.com||Lake View|township-opens"},
		{"PIN's township wins", url.Values{"email": {"ed@example.com"}, "pin": {"13331030130000"}, "township": {"worth"}},
			200, "ed@example.com|13331030130000|Jefferson|township-opens"},
		{"unknown township dropped", url.Values{"email": {"fe@example.com"}, "township": {"springfield"}},
			200, "fe@example.com|||township-opens"},
		{"display-name form rejected", url.Values{"email": {"Cy <cy@example.com>"}}, 200, ""},
		{"not an email", url.Values{"email": {"nope"}}, 200, ""},
		{"too long", url.Values{"email": {strings.Repeat("a", 250) + "@example.com"}}, 200, ""},
		{"oversized body", url.Values{"email": {"d@example.com"}, "x": {strings.Repeat("x", 5000)}}, 400, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := site.App.Exec(`DELETE FROM signup`); err != nil {
				t.Fatal(err)
			}
			code, _ := postNotify(t, ts, tt.form, nil)
			if code != tt.status {
				t.Fatalf("status %d, want %d", code, tt.status)
			}
			var got string
			err := site.App.QueryRow(`SELECT email || '|' || pin || '|' || township || '|' || reason FROM signup`).Scan(&got)
			if err != nil && tt.stored != "" {
				t.Fatalf("nothing stored: %v", err)
			}
			if got != tt.stored {
				t.Errorf("stored %q, want %q", got, tt.stored)
			}
		})
	}
	// Signing up twice for the same PIN keeps one row.
	form := url.Values{"email": {"ann@example.com"}, "pin": {"13331030130000"}}
	postNotify(t, ts, form, nil)
	postNotify(t, ts, form, nil)
	var n int
	if err := site.App.QueryRow(`SELECT COUNT(*) FROM signup WHERE email = 'ann@example.com'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("%d rows after a repeat signup (%v), want 1", n, err)
	}
}
