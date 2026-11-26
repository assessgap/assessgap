package golden_test

// The legal scope line, in one place (docs/testing.md, "compliance wording").
// AssessGap compares assessed values. It never states a market value, calls
// anything an appraisal, labels a home with a verdict or tells anyone to
// appeal, and never shows a price for itself. These tests scan everything a
// homeowner sees for every golden case: the report JSON's text, the evidence
// report's text and the website's pages.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/assessgap/assessgap/internal/cli"
	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/fixture"
	"github.com/assessgap/assessgap/internal/golden"
	"github.com/assessgap/assessgap/internal/packet"
	"github.com/assessgap/assessgap/internal/store"
	"github.com/assessgap/assessgap/internal/web"
	"github.com/assessgap/assessgap/internal/web/views"

	"github.com/a-h/templ"
)

// banned are phrases that must never appear. Each is matched case-insensitively
// unless it says otherwise.
var banned = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bmarket value`),
	regexp.MustCompile(`(?i)\bfair market\b`),
	regexp.MustCompile(`(?i)\b(would|will|could) sell for \$`),
	regexp.MustCompile(`(?i)\b(strong|weak|good|winning|solid) case\b`),
	regexp.MustCompile(`(?i)\b(you|homeowners?) should (appeal|file)\b`),
	regexp.MustCompile(`(?i)\bwe (recommend|advise)\b`),
	regexp.MustCompile(`(?i)\b(likely|sure|certain) to win\b`),
	// A price for AssessGap itself.
	regexp.MustCompile(`(?i)\bcoming soon\b`),
	regexp.MustCompile(`(?i)\bpaid us\b`),
	regexp.MustCompile(`(?i)\b(one-time|flat|our) fee\b`),
	regexp.MustCompile(`(?i)\$\d+(\.\d\d)? (report|packet|per report)\b`),
	// Lowercase "worth" (the capitalized Worth Township and city are fine).
	regexp.MustCompile(`\bworth\b`),
}

// allowedWorth are the uses of "worth" that say nothing about value.
var allowedWorth = []string{"worth double-checking", "worth a second look", "worth your time", "what my home is worth?"}

// negatable words may appear only right after a denial ("it is not an
// appraisal", "nothing here is an appraisal", "never guaranteed").
var negatable = regexp.MustCompile(`(?i)\b(apprais\w*|guarantee[ds]?)\b`)
var negation = regexp.MustCompile(`(?i)\b(not|never|isn't|nothing here is)\s+(an?\s+|be\s+)?$`)

// violations lists banned phrases in text, with some context.
func violations(text string) []string {
	text = strings.Join(strings.Fields(text), " ")
	lower := strings.ToLower(text)
	clean := text
	for _, a := range allowedWorth {
		for i := strings.Index(strings.ToLower(clean), a); i >= 0; i = strings.Index(strings.ToLower(clean), a) {
			clean = clean[:i] + strings.Repeat("_", len(a)) + clean[i+len(a):]
		}
	}
	var out []string
	ctx := func(i, j int) string { return "…" + text[max(0, i-40):min(len(text), j+40)] + "…" }
	for _, re := range banned {
		for _, m := range re.FindAllStringIndex(clean, -1) {
			out = append(out, ctx(m[0], m[1]))
		}
	}
	for _, m := range negatable.FindAllStringIndex(lower, -1) {
		if !negation.MatchString(lower[max(0, m[0]-40):m[0]]) {
			out = append(out, ctx(m[0], m[1]))
		}
	}
	return out
}

// A quoted reduction rate ("61% got a reduction", "got a reduction about 92%
// of the time") must carry the caveat close by: it is what happened to past
// appeals, never a prediction for the homeowner reading it.
var (
	rateQuote = regexp.MustCompile(`(?i)\bgot a reduction\b`)
	caveat    = regexp.MustCompile(`(?i)\b(not|aren't|isn't) a prediction for any one home\b`)
)

// caveatReach is how far after a quoted rate, in characters of visible text,
// the caveat may be: far enough for the rows of a small table, short of the
// next section.
const caveatReach = 600

// uncaveated lists quoted reduction rates in text with no caveat after them.
func uncaveated(text string) []string {
	text = strings.Join(strings.Fields(text), " ")
	var out []string
	for _, m := range rateQuote.FindAllStringIndex(text, -1) {
		if !caveat.MatchString(text[m[0]:min(len(text), m[1]+caveatReach)]) {
			out = append(out, "rate without the caveat: …"+text[max(0, m[0]-60):min(len(text), m[1]+60)]+"…")
		}
	}
	return out
}

func TestUncaveated(t *testing.T) {
	tests := []struct {
		text string
		bad  bool
	}{
		{"Homeowners like this got a reduction about 92% of the time. This is what happened to past appeals like this one; it is not a prediction for any one home.", false},
		{"61% got a reduction (170 appeals). These are outcomes for people who chose to appeal. They aren't a prediction for any one home.", false},
		{"Homeowners like this got a reduction about 92% of the time.", true},
		{"61% got a reduction. They aren't a prediction for your home.", true},
		{"It is not a prediction for any one home. 61% got a reduction.", true},
		{"Nothing quoted here.", false},
	}
	for _, tt := range tests {
		if got := len(uncaveated(tt.text)) > 0; got != tt.bad {
			t.Errorf("uncaveated(%q) = %v, want bad=%v", tt.text, uncaveated(tt.text), tt.bad)
		}
	}
}

// TestComplianceCLIText scans the terminal report ("check", no -json) for every case.
func TestComplianceCLIText(t *testing.T) {
	db := fixture.Path(t)
	for _, c := range golden.Cases {
		var out bytes.Buffer
		env := &cli.Env{Stdout: &out, Stderr: io.Discard, Getenv: func(string) string { return "" }}
		cli.Run(context.Background(), env, append([]string{"check", "-db", db}, c.Args...))
		for _, bad := range append(violations(out.String()), uncaveated(out.String())...) {
			t.Errorf("%s (%s): terminal report: %s", c.Name, c.Args[0], bad)
		}
	}
}

func TestViolations(t *testing.T) {
	tests := []struct {
		text string
		bad  bool
	}{
		{"It is not an appraisal and does not estimate what any property would sell for.", false},
		{"Worth double-checking: the record shows no full bathrooms.", false},
		{"Oak Lawn · Worth Township", false},
		{"Appeal outcomes are never guaranteed.", false},
		{"This appraisal shows your home is over-assessed.", true},
		{"Your home's market value is $300,000.", true},
		{"You have a strong case.", true},
		{"You should appeal before the deadline.", true},
		{"Your home is worth more than similar homes.", true},
		{"Results are guaranteed.", true},
		{"Download the report for $29 one-time fee", true},
		{"We don't estimate what your home would sell for, and nothing here is an appraisal.", false},
		{"is not an appraisal or an opinion of any kind", false},
		{"This is not only an appraisal but a plan.", true},
		{"No fee. This appraisal shows the gap.", true},
		{"Not sure? Results guaranteed.", true},
	}
	for _, tt := range tests {
		if got := len(violations(tt.text)) > 0; got != tt.bad {
			t.Errorf("violations(%q) = %v, want banned=%v", tt.text, violations(tt.text), tt.bad)
		}
	}
}

// TestComplianceReportJSON scans every string in "check -json" for every case.
func TestComplianceReportJSON(t *testing.T) {
	db := fixture.Path(t)
	for _, c := range golden.Cases {
		var out bytes.Buffer
		env := &cli.Env{Stdout: &out, Stderr: io.Discard, Getenv: func(string) string { return "" }}
		code := cli.Run(context.Background(), env, append([]string{"check", "-json", "-db", db}, c.Args...))
		if out.Len() == 0 {
			if code == 0 {
				t.Errorf("%s: exit 0 with no report to scan", c.Name)
			}
			continue
		}
		var v any
		if err := json.Unmarshal(out.Bytes(), &v); err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		for _, s := range stringsIn(v) {
			// Each string stands alone (the CLI and others quote reasons one at a time).
			for _, bad := range append(violations(s), uncaveated(s)...) {
				t.Errorf("%s (%s): report text: %s", c.Name, c.Args[0], bad)
			}
		}
	}
}

// TestCompliancePacket scans the evidence report text for every supported case.
func TestCompliancePacket(t *testing.T) {
	e := comps.New(fixture.Open(t))
	for _, c := range golden.Cases {
		r, err := e.Report(context.Background(), c.Args[0], 0)
		if err != nil {
			continue // unknown PINs: covered by the goldens
		}
		txt, err := packet.Text(r, generated)
		if errors.Is(err, packet.ErrUnsupported) {
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		for _, bad := range append(violations(txt), uncaveated(txt)...) {
			t.Errorf("%s (%s): evidence report: %s", c.Name, c.Args[0], bad)
		}
	}
}

// TestComplianceWebsite scans the site's pages and the report page for every case.
func TestComplianceWebsite(t *testing.T) {
	app, err := store.OpenApp(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	site, err := web.New(fixture.Open(t), app, false, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(site.Handler())
	t.Cleanup(ts.Close)

	// Every indexed page (the sitemap: the static pages, /townships and each
	// township's page), plus the report page for every case.
	// A redirect (followed to the page), both 404 pages and the search page.
	paths := []string{"/townships/lakeview", "/townships/springfield", "/no-such-page", "/search?q=2329+n+lock"}
	notFound := map[string]bool{"/townships/springfield": true, "/no-such-page": true}
	resp, err := http.Get(ts.URL + "/sitemap.xml")
	if err != nil {
		t.Fatal(err)
	}
	sitemap, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`<loc>([^<]*)</loc>`).FindAllStringSubmatch(string(sitemap), -1) {
		paths = append(paths, strings.TrimPrefix(m[1], ts.URL))
	}
	if len(paths) < 2 {
		t.Fatalf("no pages in the sitemap:\n%s", sitemap)
	}
	for _, c := range golden.Cases {
		paths = append(paths, "/check/"+c.Args[0])
	}
	tags := regexp.MustCompile(`(?s)<script.*?</script>|<style.*?</style>|<[^>]*>`)
	// Attribute text a person can see or hear: meta descriptions, tooltips, alt text, labels.
	attrs := regexp.MustCompile(`\b(?:content|title|alt|aria-label|placeholder)="([^"]*)"`)
	for _, p := range paths {
		resp, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		// Every case's page exists (unknown PINs get the 404 page, which is scanned too).
		wantStatus := 200
		if strings.HasPrefix(p, "/check/99999") || notFound[p] {
			wantStatus = 404
		}
		if resp.StatusCode != wantStatus {
			t.Errorf("%s: status %d", p, resp.StatusCode)
		}
		text := tags.ReplaceAllString(string(body), " ")
		for _, m := range attrs.FindAllStringSubmatch(string(body), -1) {
			if strings.HasPrefix(m[1], "https://") || strings.HasPrefix(m[1], "http://") {
				continue // og:url and og:image are URLs, not prose ("/townships/worth")
			}
			text += " " + m[1]
		}
		text = html.UnescapeString(text)
		for _, bad := range append(violations(text), uncaveated(text)...) {
			t.Errorf("%s: page: %s", p, bad)
		}
	}
}

// TestComplianceTownshipStates scans the township pages' copy for every window
// state. TestComplianceWebsite sees only the states the schedule has on the
// day it runs, so this renders each one, plus the signup result page a
// browser without JavaScript gets.
func TestComplianceTownshipStates(t *testing.T) {
	ctx := views.WithShare(context.Background(), views.Share{Origin: "https://assessgap.com", URL: "https://assessgap.com/townships/x"})
	appeals := &views.TownshipAppeals{Appeals: "1,000", Reduced: "60%", AvgCut: "8%", Years: "2023–2025"}
	var pages []templ.Component
	for _, st := range []string{"open", "upcoming", "closed", "unscheduled"} {
		w := views.TownshipWindow{State: st, Label: st, DaysLeft: "3 days left"}
		if st != "unscheduled" {
			w.Group, w.Open, w.Close, w.EvidenceDue = "1", "August 3, 2026", "September 1, 2026", "September 11, 2026"
		}
		d := views.TownshipData{Name: "Worth", Slug: "worth", Window: w, ScheduleAsOf: "November 24, 2026", Homes: "41,487", ProSe: appeals, Attorney: appeals}
		pages = append(pages, views.Township(views.Site{}, d))
		d.ProSe, d.Attorney = nil, nil
		pages = append(pages, views.Township(views.Site{}, d))
	}
	links := []views.TownshipLink{{Name: "Worth", Slug: "worth", Dates: "Closes September 1, 2026 (3 days left)"}}
	pages = append(pages,
		views.Townships(views.Site{}, views.TownshipIndex{Groups: []views.TownshipGroup{
			{State: "open", Heading: "Open now", Townships: links}, {State: "upcoming", Heading: "Opening soon", Townships: links},
			{State: "unscheduled", Heading: "Dates not posted yet", Townships: links}, {State: "closed", Heading: "Closed", Townships: links},
		}}),
		views.NotifyPage(views.Site{}, views.NotifyThanks()),
		views.NotifyPage(views.Site{}, views.NotifyAlready()),
		views.NotifyPage(views.Site{}, views.NotifyFormFor("", "worth", "township-opens", "That email address doesn't look right.")),
	)
	tags := regexp.MustCompile(`(?s)<script.*?</script>|<style.*?</style>|<[^>]*>`)
	for i, p := range pages {
		var b strings.Builder
		if err := p.Render(ctx, &b); err != nil {
			t.Fatal(err)
		}
		text := html.UnescapeString(tags.ReplaceAllString(b.String(), " "))
		for _, bad := range append(violations(text), uncaveated(text)...) {
			t.Errorf("township page %d: %s", i, bad)
		}
	}
}

func stringsIn(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		var out []string
		for _, e := range x {
			out = append(out, stringsIn(e)...)
		}
		return out
	case map[string]any:
		var out []string
		for _, e := range x {
			out = append(out, stringsIn(e)...)
		}
		return out
	}
	return nil
}
