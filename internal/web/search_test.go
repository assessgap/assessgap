package web

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name, in string
		n        int
		want     string
	}{
		{"short enough", "abc", 3, "abc"},
		{"ascii cut", "abcd", 3, "abc"},
		// "é" is 2 bytes (C3 A9); cutting at 2 would leave half of it.
		{"inside a 2-byte character", "aéb", 2, "a"},
		{"after a 2-byte character", "aéb", 3, "aé"},
		// "€" is 3 bytes; byte 3 is its last continuation byte.
		{"inside a 3-byte character", "a€", 3, "a"},
	}
	for _, tt := range tests {
		if got := truncate(tt.in, tt.n); got != tt.want {
			t.Errorf("%s: truncate(%q, %d) = %q, want %q", tt.name, tt.in, tt.n, got, tt.want)
		}
	}
}

func TestSearchHandler(t *testing.T) {
	ts := newTestServer(t)
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	tests := []struct {
		q        string
		htmx     bool
		status   int
		location string // redirect target, if any
		contains string
	}{
		// A full-page search with one match redirects to the report.
		{"13-33-103-013-0000", false, http.StatusSeeOther, "/check/13331030130000", ""},
		// htmx gets the results fragment, never a redirect.
		{"13-33-103-013-0000", true, http.StatusOK, "", "/check/13331030130000"},
		// A full-page search with several matches lists them.
		{"2106 n laramie", false, http.StatusOK, "", "/check/13331120411002"},
	}
	for _, tt := range tests {
		req, _ := http.NewRequest("GET", ts.URL+"/search?q="+url.QueryEscape(tt.q), nil)
		if tt.htmx {
			req.Header.Set("HX-Request", "true")
		}
		resp, err := noRedirect.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != tt.status {
			t.Errorf("%q: status %d, want %d", tt.q, resp.StatusCode, tt.status)
		}
		if loc := resp.Header.Get("Location"); loc != tt.location {
			t.Errorf("%q: Location %q, want %q", tt.q, loc, tt.location)
		}
		if !strings.Contains(string(body), tt.contains) {
			t.Errorf("%q: body lacks %q", tt.q, tt.contains)
		}
	}
}
