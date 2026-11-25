// Command check prints the comparables report for one PIN as indented JSON.
//
//	check [-year N] [-db path] <pin>
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/store"
)

func main() {
	log.SetFlags(0)
	dbPath := flag.String("db", "data/cook.db", "database built by cmd/build")
	year := flag.Int("year", 0, "tax year (default: latest year with an assessed value)")
	brief := flag.Bool("brief", false, "omit the full comp list (evidence comps are kept)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: check [-year N] [-db path] [-brief] <pin>")
		flag.PrintDefaults()
	}
	// Accept flags before or after the PIN.
	args := os.Args[1:]
	var pin string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		pin, args = args[0], args[1:]
	}
	flag.CommandLine.Parse(args)
	if pin == "" && flag.NArg() == 1 {
		pin = flag.Arg(0)
	}
	pin = normalizePIN(pin)
	if len(pin) != 14 {
		flag.Usage()
		os.Exit(2)
	}

	if _, err := os.Stat(*dbPath); err != nil {
		log.Fatal(err) // store.Open would silently create an empty DB
	}
	db, err := store.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	r, err := comps.New(db).Report(context.Background(), pin, *year)
	if err != nil {
		log.Fatal(err)
	}
	if *brief {
		r.Comps = nil
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		log.Fatal(err)
	}
}

// normalizePIN strips dashes and pads a 10-digit PIN to 14.
func normalizePIN(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "-", "")
	if len(s) == 10 {
		s += "0000"
	}
	return s
}
