package e2e

import (
	"database/sql"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/assessgap/assessgap/internal/fixture"
	"github.com/assessgap/assessgap/internal/store"
)

// noRedirect is a client that returns redirects instead of following them.
var noRedirect = &http.Client{
	Timeout:       30 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func fetch(t *testing.T, method, u string, body io.Reader, hdr map[string]string) response {
	t.Helper()
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, u, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{resp.StatusCode, resp.Header, b}
}

// TestServe starts the binary in production mode on the fixture with a
// fresh app database and walks every route a visitor or crawler reaches.
func TestServe(t *testing.T) {
	appDB := filepath.Join(t.TempDir(), "app.db")
	s := startServe(t, "-db", fixture.Path(t), "-app", appDB, "-dev=false")

	const html, pdf, text, xmlType = "text/html; charset=utf-8", "application/pdf", "text/plain; charset=utf-8", "application/xml; charset=utf-8"
	routes := []struct {
		name, path string
		status     int
		ctype      string // for 2xx and 404
		location   string // for redirects
	}{
		{"home", "/", 200, html, ""},
		{"how it works", "/how-it-works", 200, html, ""},
		{"terms", "/terms", 200, html, ""},
		{"privacy", "/privacy", 200, html, ""},
		{"townships index", "/townships", 200, html, ""},
		{"townships index, trailing slash", "/townships/", 301, "", "/townships"},
		{"township", "/townships/jefferson", 200, html, ""},
		{"township, two words", "/townships/lake-view", 200, html, ""},
		{"township variant redirects to its slug", "/townships/lakeview", 301, "", "/townships/lake-view"},
		{"township, capitalized", "/townships/Oak_Park", 301, "", "/townships/oak-park"},
		{"unknown township", "/townships/atlantis", 404, html, ""},
		{"search by PIN goes to the report", "/search?q=13331030130000", 303, "", "/check/13331030130000"},
		{"search by dashed PIN", "/search?q=13-33-103-013-0000", 303, "", "/check/13331030130000"},
		{"search by address prefix lists homes", "/search?q=2329+N+Lockwood", 303, "", "/check/13331030130000"},
		{"search with several matches", "/search?q=232", 200, html, ""},
		{"search with no match", "/search?q=1+Nowhere+St", 200, html, ""},
		{"empty search", "/search?q=", 200, html, ""},
		{"report", "/check/13331030130000", 200, html, ""},
		{"report, dashed PIN", "/check/13-33-103-013-0000", 200, html, ""},
		{"report, condo (unsupported) still has a page", "/check/13331120411001", 200, html, ""},
		{"report, malformed PIN", "/check/123", 404, html, ""},
		{"report, unknown PIN", "/check/99999999999999", 404, html, ""},
		{"evidence report PDF", "/check/13331030130000/report.pdf", 200, pdf, ""},
		{"PDF for a malformed PIN", "/check/123/report.pdf", 404, html, ""},
		{"PDF for an unsupported condo", "/check/13331120411001/report.pdf", 404, html, ""},
		{"robots.txt", "/robots.txt", 200, text, ""},
		{"sitemap", "/sitemap.xml", 200, xmlType, ""},
		{"healthz", "/healthz", 200, text, ""},
		{"stylesheet", "/static/app.css", 200, "text/css; charset=utf-8", ""},
		{"htmx", "/static/htmx.min.js", 200, "text/javascript; charset=utf-8", ""},
		{"unknown page", "/nope", 404, html, ""},
		{"unknown static file", "/static/nope.js", 404, text, ""},
	}
	for _, rt := range routes {
		t.Run(rt.name, func(t *testing.T) {
			r := fetch(t, "GET", s.base+rt.path, nil, nil)
			if r.status != rt.status {
				t.Fatalf("GET %s: %d, want %d", rt.path, r.status, rt.status)
			}
			if rt.location != "" {
				if got := r.header.Get("Location"); got != rt.location {
					t.Errorf("GET %s: Location %q, want %q", rt.path, got, rt.location)
				}
				return
			}
			if got := r.header.Get("Content-Type"); got != rt.ctype {
				t.Errorf("GET %s: Content-Type %q, want %q", rt.path, got, rt.ctype)
			}
			// Every response carries the same-origin-only policy (no third
			// parties may see a PIN), and production pages are indexable.
			if csp := r.header.Get("Content-Security-Policy"); !strings.HasPrefix(csp, "default-src 'self';") {
				t.Errorf("GET %s: Content-Security-Policy %q", rt.path, csp)
			}
			if r.header.Get("X-Content-Type-Options") != "nosniff" {
				t.Errorf("GET %s: no X-Content-Type-Options: nosniff", rt.path)
			}
			if xr := r.header.Get("X-Robots-Tag"); xr != "" {
				t.Errorf("GET %s: X-Robots-Tag %q in production mode", rt.path, xr)
			}
		})
	}

	t.Run("PDF is a whole download", func(t *testing.T) {
		r := fetch(t, "GET", s.base+"/check/13331030130000/report.pdf", nil, nil)
		if cd := r.header.Get("Content-Disposition"); cd != `attachment; filename="assessgap-13331030130000.pdf"` {
			t.Errorf("Content-Disposition %q", cd)
		}
		if _, err := parsePDF(r.body); err != nil {
			t.Errorf("report.pdf: %v", err)
		}
	})

	t.Run("versioned static assets are cached for a year", func(t *testing.T) {
		home := fetch(t, "GET", s.base+"/", nil, nil)
		css := regexp.MustCompile(`/static/app\.css\?v=[0-9a-f]+`).Find(home.body)
		if css == nil {
			t.Fatal("home page links no versioned app.css")
		}
		r := fetch(t, "GET", s.base+string(css), nil, nil)
		if r.status != 200 || r.header.Get("Cache-Control") != "public, max-age=31536000, immutable" {
			t.Errorf("GET %s: %d, Cache-Control %q", css, r.status, r.header.Get("Cache-Control"))
		}
	})

	t.Run("search, htmx fragment", func(t *testing.T) {
		r := fetch(t, "GET", s.base+"/search?q=13331030130000", nil, map[string]string{"HX-Request": "true"})
		if r.status != 200 || !strings.Contains(string(r.body), `href="/check/13331030130000"`) {
			t.Errorf("htmx search: %d, no link to the report\n%s", r.status, r.body)
		}
		if strings.Contains(string(r.body), "<html") {
			t.Error("htmx search returned a whole page")
		}
	})

	t.Run("every sitemap URL answers 200", func(t *testing.T) {
		var sm struct {
			URLs []struct {
				Loc string `xml:"loc"`
			} `xml:"url"`
		}
		if err := xml.Unmarshal(fetch(t, "GET", s.base+"/sitemap.xml", nil, nil).body, &sm); err != nil {
			t.Fatalf("sitemap.xml: %v", err)
		}
		if len(sm.URLs) == 0 {
			t.Fatal("sitemap.xml lists no URLs")
		}
		for _, u := range sm.URLs {
			p, err := url.Parse(u.Loc)
			if err != nil {
				t.Fatalf("sitemap loc %q: %v", u.Loc, err)
			}
			if r := fetch(t, "GET", s.base+p.Path, nil, nil); r.status != 200 {
				t.Errorf("sitemap URL %s: %d", p.Path, r.status)
			}
		}
	})

	const email = "e2e-signup@example.com"
	t.Run("notify signup", func(t *testing.T) {
		form := url.Values{"email": {email}, "pin": {"13331030130000"}}.Encode()
		r := fetch(t, "POST", s.base+"/notify", strings.NewReader(form),
			map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
		if r.status != 200 {
			t.Fatalf("POST /notify: %d\n%s", r.status, r.body)
		}
	})

	// The busy limiter (503 + Retry-After) is tested in internal/web; through
	// the binary on the fixture it takes ~850 simultaneous PDFs to trip, too
	// heavy for the gate (docs/dev/2026-11-27-e2e.md has the manual run).

	if err := s.stop(t); err != nil {
		t.Errorf("serve exited with %v after SIGINT, want 0\n%s", err, s.logText())
	}
	if strings.Contains(s.logText(), email) {
		t.Error("the server logged a signup email address")
	}
	app, err := store.OpenReadOnly(appDB)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	var got sql.NullString
	if err := app.QueryRow(`SELECT email FROM signup WHERE pin = '13331030130000'`).Scan(&got); err != nil || got.String != email {
		t.Errorf("signup row: %q, %v", got.String, err)
	}
}
