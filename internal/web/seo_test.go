package web

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/assessgap/assessgap/internal/web/views"
)

// TestSitemap checks that the township pages are listed under their
// canonical URLs and that report pages never are.
func TestSitemap(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	s.handleSitemap(rec, httptest.NewRequest("GET", "https://assessgap.com/sitemap.xml", nil))
	locs := map[string]bool{}
	for _, m := range regexp.MustCompile(`<loc>([^<]*)</loc>`).FindAllStringSubmatch(rec.Body.String(), -1) {
		locs[m[1]] = true
	}
	for _, want := range []string{
		"https://assessgap.com/",
		"https://assessgap.com/townships",
		"https://assessgap.com/townships/lake-view",
		"https://assessgap.com/townships/new-trier",
		"https://assessgap.com/townships/north-chicago",
		"https://assessgap.com/townships/worth",
	} {
		if !locs[want] {
			t.Errorf("sitemap lacks %s", want)
		}
	}
	for l := range locs {
		if strings.Contains(l, "/check/") {
			t.Errorf("sitemap lists a report page: %s", l)
		}
	}
}

// TestSharePreview checks the link-preview tags: a report page's preview must
// never carry its address or PIN (third parties cache previews), and indexed
// pages name their canonical URL.
func TestSharePreview(t *testing.T) {
	_, ts := newSite(t, false)
	meta := regexp.MustCompile(`<meta (?:property|name)="(?:og|twitter):[^"]*" content="([^"]*)"|<link rel="canonical" href="([^"]*)"`)
	tests := []struct {
		path      string
		private   []string // must not appear in any preview tag
		canonical string   // path, or "" for none
		ogURL     string   // path of og:url
	}{
		{"/check/13331030130000", []string{"13331030130000", "13-33-103-013-0000", "2329", "Lockwood"}, "", "/"},
		{"/check/13-33-103-013-0000", []string{"13331030130000", "13-33-103-013-0000"}, "", "/"},
		{"/townships/oak-park", nil, "/townships/oak-park", "/townships/oak-park"},
		// 404s echo the typed path, so a PIN typed there must stay out too.
		{"/townships/13331030130000", []string{"13331030130000"}, "", "/"},
		{"/CHECK/13331030130000", []string{"13331030130000"}, "", "/"},
		{"/?utm_source=x", nil, "/", "/"},
	}
	for _, tt := range tests {
		_, _, body := get(t, ts.URL+tt.path)
		var tags []string
		canonical := ""
		for _, m := range meta.FindAllStringSubmatch(string(body), -1) {
			if m[2] != "" {
				canonical = strings.TrimPrefix(m[2], ts.URL)
				continue
			}
			tags = append(tags, m[1])
		}
		if len(tags) == 0 {
			t.Errorf("%s: no preview tags", tt.path)
		}
		for _, tag := range tags {
			for _, p := range tt.private {
				if strings.Contains(strings.ToLower(tag), strings.ToLower(p)) {
					t.Errorf("%s: preview tag %q contains %q", tt.path, tag, p)
				}
			}
		}
		if canonical != tt.canonical {
			t.Errorf("%s: canonical %q, want %q", tt.path, canonical, tt.canonical)
		}
		if !strings.Contains(string(body), `property="og:url" content="`+ts.URL+tt.ogURL+`"`) {
			t.Errorf("%s: og:url is not %s", tt.path, tt.ogURL)
		}
	}
}

// TestRobots checks the one costly mistake: production must never tell
// crawlers to skip the whole site, and dev must.
func TestRobots(t *testing.T) {
	tests := []struct {
		name     string
		dev      bool
		url      string
		blockAll bool
		sitemap  string
	}{
		{"production", false, "https://assessgap.com/robots.txt", false, "Sitemap: https://assessgap.com/sitemap.xml"},
		{"local over http", false, "http://localhost:8080/robots.txt", false, "Sitemap: http://localhost:8080/sitemap.xml"},
		{"dev", true, "http://localhost:8080/robots.txt", true, ""},
	}
	for _, tt := range tests {
		s := &Server{site: views.Site{Dev: tt.dev}}
		rec := httptest.NewRecorder()
		s.handleRobots(rec, httptest.NewRequest("GET", tt.url, nil))
		body := rec.Body.String()
		if got := strings.Contains(body, "Disallow: /\n"); got != tt.blockAll {
			t.Errorf("%s: blocks everything = %v, want %v:\n%s", tt.name, got, tt.blockAll, body)
		}
		if !strings.Contains(body, tt.sitemap) {
			t.Errorf("%s: robots.txt lacks %q:\n%s", tt.name, tt.sitemap, body)
		}
	}
}
