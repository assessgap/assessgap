package web

import (
	"encoding/json"
	"html"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestHeaderNav checks that every kind of page carries the site header's
// three links and marks the current one, including on phones (the <details>
// menu repeats them).
func TestHeaderNav(t *testing.T) {
	ts := newTestServer(t)
	tests := []struct {
		path    string
		status  int
		current map[string]string // header link → aria-current; others have none
	}{
		{"/", 200, map[string]string{"/#check": "page"}},
		{"/townships", 200, map[string]string{"/townships": "page"}},
		{"/townships/jefferson", 200, map[string]string{"/townships": "true"}},
		{"/how-it-works", 200, map[string]string{"/how-it-works": "page"}},
		{"/check/13331030130000", 200, map[string]string{"/#check": "true"}},
		{"/terms", 200, nil},
		{"/no-such-page", 404, nil},
	}
	header := regexp.MustCompile(`(?s)<header class="sticky.*?</header>`)
	for _, tt := range tests {
		status, _, body := get(t, ts.URL+tt.path)
		if status != tt.status {
			t.Errorf("%s: status %d, want %d", tt.path, status, tt.status)
		}
		h := header.FindString(string(body))
		if h == "" {
			t.Errorf("%s: no site header", tt.path)
			continue
		}
		for _, link := range []string{"/#check", "/townships", "/how-it-works"} {
			tags := regexp.MustCompile(`<a href="`+regexp.QuoteMeta(link)+`"[^>]*>`).FindAllString(h, -1)
			if len(tags) != 2 {
				t.Errorf("%s: header links %s %d times, want 2 (wide nav and phone menu)", tt.path, link, len(tags))
			}
			want := tt.current[link]
			for _, tag := range tags {
				got := ""
				if m := regexp.MustCompile(`aria-current="([^"]*)"`).FindStringSubmatch(tag); m != nil {
					got = m[1]
				}
				if got != want {
					t.Errorf("%s: %s has aria-current %q, want %q", tt.path, link, got, want)
				}
			}
		}
	}
}

// TestBreadcrumbs checks the visible breadcrumbs, and that the township
// pages' BreadcrumbList data names the same pages in the same order. Report
// pages show breadcrumbs but carry no structured data (they aren't indexed).
func TestBreadcrumbs(t *testing.T) {
	ts := newTestServer(t)
	crumbs := regexp.MustCompile(`(?s)<nav aria-label="Breadcrumb".*?</nav>`)
	item := regexp.MustCompile(`(?s)<li[^>]*>\s*(?:<a href="([^"]*)"[^>]*>|<span aria-current="page"[^>]*>)([^<]*)<`)
	tests := []struct {
		path       string
		names      []string
		hrefs      []string // of every crumb but the last
		structured bool
	}{
		{"/townships", []string{"Home", "Townships"}, []string{"/"}, true},
		{"/townships/hyde-park", []string{"Home", "Townships", "Hyde Park"}, []string{"/", "/townships"}, true},
		{"/check/13331030130000", []string{"Home", "Check", "2329 N Lockwood Ave"}, []string{"/", "/search"}, false},
	}
	for _, tt := range tests {
		_, _, body := get(t, ts.URL+tt.path)
		nav := crumbs.FindString(string(body))
		var names, hrefs []string
		for _, m := range item.FindAllStringSubmatch(nav, -1) {
			names = append(names, html.UnescapeString(strings.TrimSpace(m[2])))
			if m[1] != "" {
				hrefs = append(hrefs, m[1])
			}
		}
		if !slices.Equal(names, tt.names) || !slices.Equal(hrefs, tt.hrefs) {
			t.Errorf("%s: crumbs %q %q, want %q %q", tt.path, names, hrefs, tt.names, tt.hrefs)
		}
		ld := regexp.MustCompile(`<script type="application/ld\+json">(.*?)</script>`).FindStringSubmatch(string(body))
		if (ld != nil) != tt.structured {
			t.Errorf("%s: structured data present = %v, want %v", tt.path, ld != nil, tt.structured)
		}
		if ld == nil {
			continue
		}
		var list struct {
			Items []struct {
				Position int    `json:"position"`
				Name     string `json:"name"`
				Item     string `json:"item"`
			} `json:"itemListElement"`
		}
		if err := json.Unmarshal([]byte(ld[1]), &list); err != nil {
			t.Fatalf("%s: %v", tt.path, err)
		}
		var ldNames []string
		for i, it := range list.Items {
			ldNames = append(ldNames, it.Name)
			if it.Position != i+1 {
				t.Errorf("%s: item %d has position %d", tt.path, i, it.Position)
			}
			if i < len(tt.hrefs) && it.Item != ts.URL+tt.hrefs[i] {
				t.Errorf("%s: item %d links %s, want %s", tt.path, i, it.Item, ts.URL+tt.hrefs[i])
			}
		}
		if !slices.Equal(ldNames, tt.names) {
			t.Errorf("%s: BreadcrumbList names %q, visible %q", tt.path, ldNames, tt.names)
		}
		if last := list.Items[len(list.Items)-1].Item; last != ts.URL+tt.path {
			t.Errorf("%s: last BreadcrumbList item %s, want the page itself", tt.path, last)
		}
	}
}

// TestInPageAnchors checks that every in-page link ("On this page", the
// township status jumps, the filing-steps link) lands on an element that
// exists, for report shapes with and without the optional sections.
func TestInPageAnchors(t *testing.T) {
	ts := newTestServer(t)
	href := regexp.MustCompile(`href="(/[^"#]*)?#([^"]+)"`)
	for _, tt := range []struct {
		path  string
		wants []string // anchors that must be linked from this page
	}{
		{"/check/13331030130000", []string{"summary", "numbers", "lower", "appeals", "options", "homes"}}, // evidence to list
		{"/check/13331020180000", []string{"summary", "numbers", "appeals", "options", "homes"}},          // lowest tier
		{"/how-it-works", []string{"compare", "tested", "dont", "faq"}},
		{"/townships", nil},
		{"/townships/jefferson", nil},
	} {
		_, _, body := get(t, ts.URL+tt.path)
		page := string(body)
		linked := map[string]bool{}
		for _, m := range href.FindAllStringSubmatch(page, -1) {
			target := page
			if m[1] != "" && m[1] != tt.path {
				if !strings.HasPrefix(m[1], "/townships/") {
					continue // /#check is the home page's; checked by its own page
				}
				_, _, other := get(t, ts.URL+m[1])
				target = string(other)
			} else {
				linked[m[2]] = true
			}
			if !strings.Contains(target, `id="`+m[2]+`"`) {
				t.Errorf("%s: link %s#%s has no target", tt.path, m[1], m[2])
			}
		}
		for _, w := range tt.wants {
			if !linked[w] {
				t.Errorf("%s: no in-page link to #%s", tt.path, w)
			}
		}
	}
}

// TestTownshipWayfinding checks the routes between townships: the index's
// A–Z list, each page's previous and next township (wrapping at the ends),
// and the township 404, which links the index and every township.
func TestTownshipWayfinding(t *testing.T) {
	ts := newTestServer(t)
	links := func(body []byte) map[string]int {
		n := map[string]int{}
		for _, m := range regexp.MustCompile(`href="(/townships/[a-z-]+)"`).FindAllStringSubmatch(string(body), -1) {
			n[m[1]]++
		}
		return n
	}
	_, _, index := get(t, ts.URL+"/townships")
	if n := len(links(index)); n != 38 {
		t.Errorf("/townships links %d distinct townships, want 38", n)
	}
	for _, tt := range []struct{ path, prev, next string }{
		{"/townships/hyde-park", "/townships/hanover", "/townships/jefferson"},
		{"/townships/barrington", "/townships/worth", "/townships/berwyn"},
		{"/townships/worth", "/townships/wheeling", "/townships/barrington"},
	} {
		_, _, body := get(t, ts.URL+tt.path)
		for rel, want := range map[string]string{"prev": tt.prev, "next": tt.next} {
			if !strings.Contains(string(body), `href="`+want+`" rel="`+rel+`"`) {
				t.Errorf("%s: no rel=%s link to %s", tt.path, rel, want)
			}
		}
	}
	status, _, body := get(t, ts.URL+"/townships/springfield")
	if status != 404 {
		t.Errorf("unknown township: status %d", status)
	}
	if !strings.Contains(string(body), `<a href="/townships" class="text-emerald-700`) || len(links(body)) != 38 {
		t.Errorf("township 404 should link /townships and all 38 townships (links %d)", len(links(body)))
	}
	status, _, body = get(t, ts.URL+"/no-such-page")
	if status != 404 || !strings.Contains(string(body), `action="/search"`) ||
		!regexp.MustCompile(`<main[^>]*>[\s\S]*<a href="/" class=`).Match(body) {
		t.Errorf("generic 404 should offer the search and a link home (status %d)", status)
	}
}
