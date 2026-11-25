// Command packet renders the Assessment Evidence Report PDF for one PIN.
//
//	packet [-year N] [-db path] -o out.pdf <pin>   (flags go before the PIN)
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/packet"
	"github.com/assessgap/assessgap/internal/store"
)

func main() {
	log.SetFlags(0)
	dbPath := flag.String("db", "data/cook.db", "database built by cmd/build")
	year := flag.Int("year", 0, "tax year (default: latest year with an assessed value)")
	out := flag.String("o", "", "output PDF path (required)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: packet <pin> -o out.pdf [-year N] [-db path]")
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
	pin = strings.ReplaceAll(strings.TrimSpace(pin), "-", "")
	if len(pin) == 10 {
		pin += "0000"
	}
	if len(pin) != 14 || *out == "" {
		flag.Usage()
		os.Exit(2)
	}

	if _, err := os.Stat(*dbPath); err != nil {
		log.Fatal(err)
	}
	db, err := store.OpenReadOnly(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	r, err := comps.New(db).Report(context.Background(), pin, *year)
	if err != nil {
		log.Fatal(err)
	}
	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	if err := packet.Render(f, r, time.Now()); err != nil {
		f.Close()
		os.Remove(*out)
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
	fmt.Println(*out)
}
