package web

import (
	"net/http"
	"testing"
)

// TestTownshipRoutes checks status codes and that variants of a township's
// name redirect permanently to its one canonical URL.
func TestTownshipRoutes(t *testing.T) {
	ts := newTestServer(t)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	tests := []struct {
		name, path string
		status     int
		location   string
	}{
		{"index", "/townships", 200, ""},
		{"canonical slug", "/townships/lake-view", 200, ""},
		{"Lake is not Lake View", "/townships/lake", 200, ""},
		{"no hyphen", "/townships/lakeview", 301, "/townships/lake-view"},
		{"capitals and underscore", "/townships/Lake_View", 301, "/townships/lake-view"},
		{"township suffix", "/townships/new-trier-township", 301, "/townships/new-trier"},
		{"encoded space", "/townships/Hyde%20Park", 301, "/townships/hyde-park"},
		{"unknown township", "/townships/springfield", 404, ""},
		{"empty slug", "/townships/", 301, "/townships"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := client.Get(ts.URL + tt.path)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != tt.status || resp.Header.Get("Location") != tt.location {
				t.Errorf("%s: %d %q, want %d %q", tt.path, resp.StatusCode, resp.Header.Get("Location"), tt.status, tt.location)
			}
		})
	}
}
