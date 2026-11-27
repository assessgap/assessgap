package web

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/assessgap/assessgap/internal/fixture"
	"github.com/assessgap/assessgap/internal/store"
)

// fuzzHandler serves the fixture in production mode, for driving routes with
// arbitrary input in-process (no sockets, so the fuzzer runs fast).
func fuzzHandler(f *testing.F) http.Handler {
	f.Helper()
	app, err := store.OpenApp(filepath.Join(f.TempDir(), "app.db"))
	if err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() { app.Close() })
	site, err := New(fixture.Open(f), app, false, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		f.Fatal(err)
	}
	return site.Handler()
}

func serve(h http.Handler, path, rawQuery string, header http.Header) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "/", nil)
	r.URL = &url.URL{Path: path, RawPath: "", RawQuery: rawQuery}
	r.RequestURI = r.URL.RequestURI()
	for k, v := range header {
		r.Header[k] = v
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// FuzzTownshipSlug drives /townships/{slug} with arbitrary segments. Every
// answer is the page, a 404, or a 301 to the township's canonical page, and
// that page must itself answer 200 (a redirect to a 404 or a redirect loop
// strands someone following a link to their township).
func FuzzTownshipSlug(f *testing.F) {
	for _, s := range []string{
		"jefferson", "Jefferson", "lake-view", "lakeview", "Lake_View", "lake+view", "lake-view-township",
		"north-chicago", "nope", "", "-", "..", "%2F", "évanston", "oak park", "OAK-PARK ", "lake-view/",
	} {
		f.Add(s)
	}
	h := fuzzHandler(f)
	f.Fuzz(func(t *testing.T, seg string) {
		if strings.ContainsRune(seg, '/') {
			return // a different route
		}
		name, canonical, ok := resolveTownship(seg)
		if canonical && seg != townshipSlug(name) {
			t.Fatalf("resolveTownship(%q) = %q canonical, but its slug is %q", seg, name, townshipSlug(name))
		}
		w := serve(h, "/townships/"+seg, "", nil)
		switch w.Code {
		case http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
			// ServeMux cleaning "." or ".." segments; only check it stays on-site.
			if loc := w.Header().Get("Location"); !strings.HasPrefix(loc, "/") || strings.HasPrefix(loc, "//") {
				t.Fatalf("/townships/%q: mux redirects off-site: %q", seg, loc)
			}
		case http.StatusOK, http.StatusNotFound:
			if ok != (w.Code == http.StatusOK) && seg != "" {
				t.Fatalf("/townships/%q: %d, resolveTownship ok=%v", seg, w.Code, ok)
			}
		case http.StatusMovedPermanently:
			loc := w.Header().Get("Location")
			if !strings.HasPrefix(loc, "/townships") || strings.HasPrefix(loc, "//") {
				t.Fatalf("/townships/%q redirects off the townships pages: %q", seg, loc)
			}
			u, err := url.Parse(loc)
			if err != nil {
				t.Fatalf("/townships/%q: bad Location %q: %v", seg, loc, err)
			}
			if got := serve(h, u.Path, "", nil); got.Code != http.StatusOK {
				t.Fatalf("/townships/%q redirects to %q, which answers %d", seg, loc, got.Code)
			}
		default:
			t.Fatalf("/townships/%q: status %d", seg, w.Code)
		}
	})
}

// FuzzSearchRoute drives /search, plain and htmx, with arbitrary queries.
// Bad input is the visitor's problem, never a server error.
func FuzzSearchRoute(f *testing.F) {
	for _, s := range []string{
		"13331030130000", "13-33-103-013-0000", "3134 N Luna Ave", "luna", "", "a", "%", "_", "'", "\x00",
		strings.Repeat("9", 500), "1333103013", "5555 belmont #2, chicago", "ü",
	} {
		f.Add(s, false)
	}
	h := fuzzHandler(f)
	f.Fuzz(func(t *testing.T, q string, htmx bool) {
		hdr := http.Header{}
		if htmx {
			hdr.Set("HX-Request", "true")
		}
		w := serve(h, "/search", url.Values{"q": {q}}.Encode(), hdr)
		if w.Code >= 500 {
			t.Fatalf("/search?q=%q (htmx %v): %d\n%s", q, htmx, w.Code, w.Body.String())
		}
	})
}

// FuzzCheckRoute drives /check/{pin} and its PDF with arbitrary segments.
func FuzzCheckRoute(f *testing.F) {
	for _, s := range []string{
		"13331030130000", "13-33-103-013-0000", "1333103013", "99999999999999", "123", "", "abc", "0",
		"13331120411001", "24294030030000",
	} {
		f.Add(s)
	}
	h := fuzzHandler(f)
	f.Fuzz(func(t *testing.T, seg string) {
		if seg == "" || strings.ContainsRune(seg, '/') {
			return // a different route
		}
		for _, p := range []string{"/check/" + seg, "/check/" + seg + "/report.pdf"} {
			w := serve(h, p, "", nil)
			if w.Code >= 500 {
				t.Fatalf("%q: %d\n%s", p, w.Code, w.Body.String())
			}
			if w.Code == http.StatusFound || w.Code == http.StatusMovedPermanently {
				if loc := w.Header().Get("Location"); !strings.HasPrefix(loc, "/") || strings.HasPrefix(loc, "//") {
					t.Fatalf("%q redirects off-site: %q", p, loc)
				}
			}
		}
	})
}
