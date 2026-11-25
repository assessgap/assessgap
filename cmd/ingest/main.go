// Command ingest downloads Cook County open data and loads it into SQLite.
//
//	ingest fetch [dataset...]   download paged CSVs into data/raw/<name>/
//	ingest load  [dataset...]   (re)build raw_<name> tables in data/cook.db
//	ingest all   [dataset...]   fetch then load
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/assessgap/assessgap/internal/socrata"
	"github.com/assessgap/assessgap/internal/store"
)

// Cook County datasets. Year filters keep the DB to what comps/stats need.
var datasets = []socrata.Dataset{
	{Name: "parcel_universe", ID: "pabr-t5kh"},
	{Name: "addresses", ID: "3723-97qp", Where: "year >= '2025'"},
	{Name: "assessed_values", ID: "uzyt-m557", Where: "year >= '2022'"},
	{Name: "sales", ID: "wvhk-k5uv", Where: "year >= '2018'"},
	{Name: "res_chars", ID: "x54s-btds", Where: "year >= '2025'"},
	{Name: "condo_chars", ID: "3r7i-mrz4", Where: "year >= '2025'"},
	{Name: "bor_appeals", ID: "7pny-nedm", Where: "tax_year >= '2021'"},
	{Name: "ccao_appeals", ID: "y282-6ig3", Where: "year >= '2021'"},
}

func main() {
	dataDir := flag.String("data", "data", "data directory")
	rawFlag := flag.String("raw", "", "directory for downloaded pages (default <data>/raw). Fetch skips pages already there, so use a fresh dir for a refresh")
	dbFlag := flag.String("db", "", "database to load into (default <data>/cook.db)")
	pageSize := flag.Int("page", 50000, "rows per request")
	pinPrefix := flag.String("pin-prefix", "", "only rows whose PIN starts with this (for small dev samples)")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: ingest [flags] fetch|load|all [dataset...]")
		os.Exit(2)
	}
	cmd, names := flag.Arg(0), flag.Args()[1:]
	sel := datasets
	if len(names) > 0 {
		sel = nil
		for _, n := range names {
			found := false
			for _, d := range datasets {
				if d.Name == n {
					sel, found = append(sel, d), true
				}
			}
			if !found {
				log.Fatalf("unknown dataset %q", n)
			}
		}
	}
	if *pinPrefix != "" {
		for i := range sel {
			cond := fmt.Sprintf("starts_with(pin, '%s')", *pinPrefix)
			if sel[i].Where != "" {
				cond = "(" + sel[i].Where + ") AND " + cond
			}
			sel[i].Where = cond
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logf := func(f string, a ...any) { log.Printf(f, a...) }
	client := &socrata.Client{
		Domain:   "datacatalog.cookcountyil.gov",
		AppToken: os.Getenv("SOCRATA_APP_TOKEN"),
		PageSize: *pageSize,
		HTTP:     &http.Client{Timeout: 5 * time.Minute},
		Log:      logf,
	}
	rawDir, dbPath := *rawFlag, *dbFlag
	if rawDir == "" {
		rawDir = filepath.Join(*dataDir, "raw")
	}
	if dbPath == "" {
		dbPath = filepath.Join(*dataDir, "cook.db")
	}

	if cmd == "fetch" || cmd == "all" {
		for _, d := range sel {
			t := time.Now()
			n, err := client.Fetch(ctx, d, filepath.Join(rawDir, d.Name))
			if err != nil {
				log.Fatalf("fetch %s: %v", d.Name, err)
			}
			log.Printf("%s: %d pages fetched in %s", d.Name, n, time.Since(t).Round(time.Second))
		}
	}
	if cmd == "load" || cmd == "all" {
		db, err := store.Open(dbPath)
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		for _, d := range sel {
			t := time.Now()
			if _, err := store.LoadRaw(db, d.Name, filepath.Join(rawDir, d.Name), logf); err != nil {
				log.Fatalf("load %s: %v", d.Name, err)
			}
			log.Printf("%s: loaded in %s", d.Name, time.Since(t).Round(time.Second))
		}
	}
}
