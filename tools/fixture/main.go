// Command fixture extracts the frozen test fixture in testdata/fixture from a
// full county database built by "assessgap db sync".
//
// It opens the source database read-only and, for every ingested dataset,
// writes the columns build.sql reads (ingest.Columns) for every PIN in the
// chosen Assessor neighborhoods, sorted, as one CSV page per dataset. Tests
// load those pages with the same code as a real build (store.LoadRaw, then
// ingest.Build), so the fixture exercises build.sql too.
//
//	go run ./tools/fixture -src data/cook.db -out testdata/fixture -nbhd 27080,39082,71280
//
// A rebuild changes the goldens; see testdata/README.md.
package main

import (
	"database/sql"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/assessgap/assessgap/internal/ingest"
	"github.com/assessgap/assessgap/internal/store"
)

// scrub replaces columns that can hold a person's name. Nothing in build.sql
// or the engine reads their content, so a placeholder keeps the row shape.
var scrub = map[string]map[string]string{
	"ccao_appeals": {"agent_name": "AGENT"}, // attorney or agent names
}

var nbhdRe = regexp.MustCompile(`^[0-9]{5}$`)

func main() {
	src := flag.String("src", "data/cook.db", "full county database (opened read-only)")
	out := flag.String("out", "testdata/fixture", "output directory")
	nbhds := flag.String("nbhd", "27080,39082,71280", "comma-separated Assessor neighborhood codes")
	flag.Parse()
	log.SetFlags(0)

	codes := strings.Split(*nbhds, ",")
	for _, c := range codes {
		if !nbhdRe.MatchString(c) {
			log.Fatalf("fixture: neighborhood %q is not 5 digits", c)
		}
	}
	db, err := store.OpenReadOnly(*src)
	if err != nil {
		log.Fatalf("fixture: %v", err)
	}
	defer db.Close()

	in := "('" + strings.Join(codes, "','") + "')"
	pins := "SELECT pin FROM raw_parcel_universe WHERE nbhd_code IN " + in +
		" UNION SELECT pin FROM raw_assessed_values WHERE nbhd IN " + in

	names := make([]string, 0, len(ingest.Datasets))
	for _, d := range ingest.Datasets {
		names = append(names, d.Name)
	}
	sort.Strings(names)
	for _, name := range names {
		n, err := extract(db, name, pins, filepath.Join(*out, name))
		if err != nil {
			log.Fatalf("fixture: %s: %v", name, err)
		}
		log.Printf("%-16s %7d rows", name, n)
	}
}

// extract writes dir/page-00000.csv with the dataset's rows for the PINs
// selected by pinsSQL, ordered by every column so the output is stable.
func extract(db *sql.DB, name, pinsSQL, dir string) (int, error) {
	cols := ingest.Columns[name]
	order := make([]string, len(cols))
	for i := range cols {
		order[i] = fmt.Sprint(i + 1)
	}
	q := fmt.Sprintf(`SELECT "%s" FROM raw_%s WHERE pin IN (%s) ORDER BY %s`,
		strings.Join(cols, `", "`), name, pinsSQL, strings.Join(order, ", "))
	rows, err := db.Query(q)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	f, err := os.Create(filepath.Join(dir, "page-00000.csv"))
	if err != nil {
		return 0, err
	}
	w := csv.NewWriter(f)
	if err := w.Write(cols); err != nil {
		f.Close()
		return 0, err
	}
	vals := make([]sql.NullString, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	rec := make([]string, len(cols))
	n := 0
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			f.Close()
			return n, err
		}
		for i, v := range vals {
			rec[i] = v.String
			if r, ok := scrub[name][cols[i]]; ok && v.String != "" {
				rec[i] = r
			}
		}
		if err := w.Write(rec); err != nil {
			f.Close()
			return n, err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		f.Close()
		return n, err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return n, err
	}
	return n, f.Close()
}
