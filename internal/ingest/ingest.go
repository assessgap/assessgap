// Package ingest downloads Cook County open data and builds the typed,
// indexed SQLite database the comparison engine reads.
//
// The pipeline has three steps: Fetch writes paged CSVs to disk, Load copies
// them into raw_<name> tables, and Build turns those into typed tables
// (build.sql).
package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/assessgap/assessgap/internal/socrata"
	"github.com/assessgap/assessgap/internal/store"
)

// Domain is the Cook County open data portal.
const Domain = "datacatalog.cookcountyil.gov"

// Datasets are the Cook County datasets we ingest. Year filters keep the
// database to what comparisons and neighborhood statistics need.
var Datasets = []socrata.Dataset{
	{Name: "parcel_universe", ID: "pabr-t5kh"},
	{Name: "addresses", ID: "3723-97qp", Where: "year >= '2025'"},
	{Name: "assessed_values", ID: "uzyt-m557", Where: "year >= '2022'"},
	{Name: "sales", ID: "wvhk-k5uv", Where: "year >= '2018'"},
	{Name: "res_chars", ID: "x54s-btds", Where: "year >= '2025'"},
	{Name: "condo_chars", ID: "3r7i-mrz4", Where: "year >= '2025'"},
	{Name: "bor_appeals", ID: "7pny-nedm", Where: "tax_year >= '2021'"},
	{Name: "ccao_appeals", ID: "y282-6ig3", Where: "year >= '2021'"},
}

// Select returns the named datasets (all of them when names is empty), each
// restricted to PINs starting with pinPrefix when it is set.
func Select(names []string, pinPrefix string) ([]socrata.Dataset, error) {
	var sel []socrata.Dataset
	if len(names) == 0 {
		sel = append(sel, Datasets...)
	}
	for _, n := range names {
		found := false
		for _, d := range Datasets {
			if d.Name == n {
				sel, found = append(sel, d), true
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown dataset %q", n)
		}
	}
	if pinPrefix != "" {
		for _, c := range pinPrefix {
			if c < '0' || c > '9' {
				return nil, fmt.Errorf("pin prefix %q: digits only", pinPrefix)
			}
		}
		for i := range sel {
			sel[i].Where = And(sel[i].Where, fmt.Sprintf("starts_with(pin, '%s')", pinPrefix))
		}
	}
	return sel, nil
}

// And joins two SoQL conditions; either may be empty.
func And(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return "(" + a + ") AND (" + b + ")"
}

// NewClient returns a Socrata client for the county portal. SOCRATA_APP_TOKEN,
// if set, raises the rate limits.
func NewClient(pageSize int, log func(string, ...any)) *socrata.Client {
	return &socrata.Client{
		Domain:   Domain,
		AppToken: os.Getenv("SOCRATA_APP_TOKEN"),
		PageSize: pageSize,
		HTTP:     &http.Client{Timeout: 5 * time.Minute},
		Log:      log,
	}
}

// Fetch downloads each dataset into rawDir/<name>/. Pages already on disk are
// skipped, so use a fresh rawDir for a refresh.
func Fetch(ctx context.Context, c *socrata.Client, sel []socrata.Dataset, rawDir string, log func(string, ...any)) error {
	for _, d := range sel {
		t := time.Now()
		n, err := c.Fetch(ctx, d, filepath.Join(rawDir, d.Name))
		if err != nil {
			return fmt.Errorf("fetch %s: %w", d.Name, err)
		}
		log("%s: %d pages fetched in %s", d.Name, n, time.Since(t).Round(time.Second))
	}
	return nil
}

// Load replaces raw_<name> in db with each dataset's pages from rawDir.
func Load(db *sql.DB, sel []socrata.Dataset, rawDir string, log func(string, ...any)) error {
	for _, d := range sel {
		t := time.Now()
		if _, err := store.LoadRaw(db, d.Name, filepath.Join(rawDir, d.Name), log); err != nil {
			return fmt.Errorf("load %s: %w", d.Name, err)
		}
		log("%s: loaded in %s", d.Name, time.Since(t).Round(time.Second))
	}
	return nil
}
