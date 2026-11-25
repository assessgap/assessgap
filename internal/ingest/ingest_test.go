package ingest_test

import (
	"os"
	"testing"

	"github.com/assessgap/assessgap/internal/fixture"
	"github.com/assessgap/assessgap/internal/ingest"
)

func TestMain(m *testing.M) { os.Exit(fixture.Main(m)) }

func TestSelect(t *testing.T) {
	tests := []struct {
		name    string
		names   []string
		prefix  string
		wantErr bool
		where   string // of the one dataset selected
	}{
		{"prefix only", []string{"parcel_universe"}, "1328", false, "starts_with(pin, '1328')"},
		{"prefix joined to the year filter", []string{"sales"}, "1328", false, "(year >= '2018') AND (starts_with(pin, '1328'))"},
		{"unknown dataset", []string{"nope"}, "", true, ""},
		// The prefix is pasted into SoQL, so anything but digits is refused.
		{"non-digit prefix", nil, "13'; --", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ingest.Select(tt.names, tt.prefix)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && (len(got) != 1 || got[0].Where != tt.where) {
				t.Errorf("got %+v, want one dataset with Where %q", got, tt.where)
			}
		})
	}
}

// TestBuildSaleFilters checks build.sql keeps only arm's-length sales. The
// report lists a home's recorded sales, so a filtered-out sale must not appear.
// Document numbers are from the fixture (testdata/fixture/sales).
func TestBuildSaleFilters(t *testing.T) {
	db := fixture.Open(t)
	tests := []struct {
		name string
		doc  string
		kept bool
	}{
		{"multi-parcel sale", "2229310004", false},
		{"same sale within 365 days", "1912234080", false},
		{"under $10,000", "1901416085", false},
		{"non-qualifying deed type", "1803708185", false},
		{"clean sale", "2211801365", true},
	}
	for _, tt := range tests {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sale WHERE doc_no = ?`, tt.doc).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if kept := n > 0; kept != tt.kept {
			t.Errorf("%s (doc %s): kept = %v, want %v", tt.name, tt.doc, kept, tt.kept)
		}
	}
	// 1,510 raw sale rows in the fixture; 1,119 pass every filter.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sale`).Scan(&n); err != nil || n != 1119 {
		t.Errorf("%d sales kept (%v), want 1119", n, err)
	}
}
