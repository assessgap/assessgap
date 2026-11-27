package socrata

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"path/filepath"
	"testing"
)

// pagesAPI serves a fixed body for $offset=0 and a header-only page after.
type pagesAPI struct{ first []byte }

func (a pagesAPI) RoundTrip(r *http.Request) (*http.Response, error) {
	body := []byte("pin\n")
	if r.URL.Query().Get("$offset") == "0" {
		body = a.first
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}, Request: r}, nil
}

// FuzzFetchPage downloads one arbitrary CSV page as Socrata would send it.
// Fetch must count the page's rows the way load reads them, report the
// pages it kept, and keep a first page that has a header even with no rows
// (load builds an empty table from it; without it, a small -pin-prefix
// build stopped at "no pages"). Seeds in testdata/fuzz/FuzzFetchPage.
func FuzzFetchPage(f *testing.F) {
	for _, s := range []string{
		"pin,year\n13331030130000,2026\n", "pin,year\n", "\"pin\",\"year\"\n", "", "pin\n\"a\nb\"\n",
		"pin,year\n1,2\n3\n", "pin\n\"unterminated\n", "\n\n\n",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		dir := t.TempDir()
		c := &Client{Domain: "county.test", PageSize: 3, HTTP: &http.Client{Transport: pagesAPI{body}},
			Log: func(string, ...any) {}}
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // a page that fails to parse is retried after a wait; don't wait
		n, err := c.Fetch(ctx, Dataset{Name: "x", ID: "abcd-1234"}, filepath.Join(dir, "x"))
		if err != nil {
			return // malformed CSV: Fetch refuses it rather than keeping it
		}
		pages, _ := filepath.Glob(filepath.Join(dir, "x", "page-*.csv"))
		if len(pages) != n {
			t.Fatalf("Fetch reported %d pages, kept %d", n, len(pages))
		}
		rows, err := countRows(filepath.Join(dir, "x", "page-00000.csv"))
		if len(pages) > 0 && err != nil {
			t.Fatalf("kept a page it can't read: %v", err)
		}
		hasHeader := false
		if _, err := NewCSVReader(bytes.NewReader(body)).Read(); err == nil {
			hasHeader = true
		}
		if hasHeader && n == 0 {
			t.Fatalf("a first page with a header was dropped (%d rows)", rows)
		}
	})
}
