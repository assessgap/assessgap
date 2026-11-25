package web

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestLoadTownshipFacts checks the aggregation across neighborhoods on a
// hand-built database. The fixture has one neighborhood per township, so the
// goldens can't tell a reduction-weighted average from a plain one.
func TestLoadTownshipFacts(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
		CREATE TABLE parcel (pin TEXT, class TEXT, township_code TEXT, township_name TEXT, nbhd TEXT);
		INSERT INTO parcel VALUES ('1','203','39','Worth','39010'), ('2','299','39','Worth','39010'),
			('3','278','39','Worth','39020'), ('4',NULL,'40','Nowhere','40010');
		CREATE TABLE nbhd_bor_stats (nbhd TEXT, pro_se INTEGER, n_appeals INTEGER, n_decrease INTEGER,
			avg_cut_pct REAL, first_year INTEGER, last_year INTEGER);
		INSERT INTO nbhd_bor_stats VALUES
			('39010', 1, 10, 8, 0.10, 2023, 2025),
			('39020', 1, 30, 2, 0.20, 2024, 2025),
			('39030', 1, 5, 0, NULL, 2023, 2024),
			('99999', 1, 50, 50, 0.50, 2023, 2025);`)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := loadTownshipFacts(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	w := facts["Worth"]
	// Homes: classes 203 and 278 are checked, 299 isn't. Appeals: 10+30+5 = 45,
	// reduced 8+2 = 10 (22%). Average cut weighted by reductions:
	// (0.10*8 + 0.20*2) / 10 = 0.12, not the plain mean 0.15. 99999 is no township's.
	got := appealsView(w.proSe, w.hasProSe)
	if w.homes != 2 || got.Appeals != "45" || got.Reduced != "22%" || got.AvgCut != "12%" || got.Years != "2023–2025" {
		t.Errorf("Worth: homes %d, %+v; want 2 homes, 45 appeals, 22%% reduced, 12%% average cut, 2023–2025", w.homes, *got)
	}
	if n := facts["Nowhere"]; !n.found || n.homes != 0 {
		t.Errorf("a township with no classed parcels: %+v, want found with 0 homes", n)
	}
}

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
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tt.status || resp.Header.Get("Location") != tt.location {
				t.Errorf("%s: %d %q, want %d %q", tt.path, resp.StatusCode, resp.Header.Get("Location"), tt.status, tt.location)
			}
			// A 200 page is the township it names, not a near match ("lake" vs "lake-view").
			if tt.status == 200 && tt.path != "/townships" && !strings.Contains(string(body), `rel="canonical" href="`+ts.URL+tt.path+`"`) {
				t.Errorf("%s: page is not that township's canonical page", tt.path)
			}
		})
	}
}
