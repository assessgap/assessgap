package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/assessgap/assessgap/internal/web/views"
)

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
