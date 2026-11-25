package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/assessgap/assessgap/internal/web/views"
)

func TestRobotsAndSitemap(t *testing.T) {
	prod := &Server{site: views.Site{}}
	rec := httptest.NewRecorder()
	prod.handleRobots(rec, httptest.NewRequest("GET", "https://assessgap.com/robots.txt", nil))
	body := rec.Body.String()
	for _, want := range []string{"Disallow: /search", "Disallow: /notify", "Sitemap: https://assessgap.com/sitemap.xml"} {
		if !strings.Contains(body, want) {
			t.Errorf("robots.txt missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Disallow: /\n") {
		t.Errorf("production robots.txt blocks everything:\n%s", body)
	}

	dev := &Server{site: views.Site{Dev: true}}
	rec = httptest.NewRecorder()
	dev.handleRobots(rec, httptest.NewRequest("GET", "http://localhost:8080/robots.txt", nil))
	if !strings.Contains(rec.Body.String(), "Disallow: /\n") {
		t.Errorf("dev robots.txt should block everything:\n%s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	prod.handleSitemap(rec, httptest.NewRequest("GET", "https://assessgap.com/sitemap.xml", nil))
	sm := rec.Body.String()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/xml") {
		t.Errorf("sitemap content type %q", ct)
	}
	for _, want := range []string{"<loc>https://assessgap.com/</loc>", "<loc>https://assessgap.com/how-it-works</loc>"} {
		if !strings.Contains(sm, want) {
			t.Errorf("sitemap missing %q:\n%s", want, sm)
		}
	}
	if strings.Contains(sm, "/check/") {
		t.Errorf("sitemap lists report pages:\n%s", sm)
	}
}
