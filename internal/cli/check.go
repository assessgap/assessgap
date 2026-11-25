package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/search"
)

func runCheck(ctx context.Context, env *Env, args []string) error {
	fs := newFlags(env, "check", "[-db path] [-year N] [-json] <address-or-PIN>")
	db := fs.String("db", "", "county database (default $ASSESSGAP_DB, else "+DefaultDB+")")
	year := fs.Int("year", 0, "tax year (default: latest year with an assessed value)")
	live := fs.Bool("live", false, "fetch this home's data from the county's open data API instead of a local database (PIN only)")
	asJSON := fs.Bool("json", false, "print the full report as JSON")
	brief := fs.Bool("brief", false, "with -json, omit the full list of similar homes (lower-assessed ones are kept)")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	q := joinArgs(pos)
	if q == "" {
		fs.Usage()
		return errUsage
	}
	r, err := lookup(ctx, env, *db, *live, q, *year)
	if err != nil {
		return err
	}
	if *asJSON {
		if *brief {
			r.Comps = nil
		}
		enc := json.NewEncoder(env.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	}
	return writeText(env.Stdout, r, time.Now())
}

// lookup opens the database (or, with live, fetches one PIN's data from the
// API), resolves q to one PIN and builds its report.
func lookup(ctx context.Context, env *Env, dbFlag string, live bool, q string, year int) (*comps.Report, error) {
	path := dbPath(env, dbFlag)
	if live || useLiveByDefault(env, dbFlag, path, q) {
		return lookupLive(ctx, env, q, year)
	}
	db, err := openCounty(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	pin, err := resolve(ctx, env, db, q)
	if err != nil {
		return nil, err
	}
	r, err := comps.New(db).Report(ctx, pin, year)
	if errors.Is(err, comps.ErrNotFound) {
		if year != 0 {
			return nil, fmt.Errorf("PIN %s has no assessed value on record for tax year %d", search.FormatPIN(pin), year)
		}
		return nil, fmt.Errorf("PIN %s has no assessed value on record", search.FormatPIN(pin))
	}
	return r, err
}

// resolve turns a PIN or an address into one PIN. A PIN is used as typed. An
// address must match one parcel, or exactly one parcel's full address; if it
// matches several, they are listed on stderr and resolve fails with status 1.
func resolve(ctx context.Context, env *Env, db *sql.DB, q string) (string, error) {
	if pin, ok := search.ParsePIN(q); ok {
		return pin, nil
	}
	ms, err := search.Find(ctx, db, q)
	if err != nil {
		return "", err
	}
	if len(ms) == 1 {
		return ms[0].PIN, nil
	}
	if m, ok := search.Exact(q, ms); ok {
		return m.PIN, nil
	}
	if len(ms) == 0 {
		return "", &exitError{1, fmt.Sprintf("no Cook County address starts with %q. Try the street number and name, like \"1234 W Main\", or a 14-digit PIN.", q)}
	}
	more := ""
	if len(ms) == search.MaxMatches {
		more = " (showing the first " + fmt.Sprint(search.MaxMatches) + ")"
	}
	fmt.Fprintf(env.Stderr, "%q matches several properties%s. Run again with a PIN or a fuller address:\n", q, more)
	tw := tabwriter.NewWriter(env.Stderr, 0, 0, 2, ' ', 0)
	for _, m := range ms {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", search.FormatPIN(m.PIN), m.Address, m.City, m.Kind())
	}
	tw.Flush()
	return "", &exitError{code: 1}
}

// writeText prints a readable report for a terminal.
func writeText(w io.Writer, r *comps.Report, now time.Time) error {
	p := &printer{w: w}
	addr := titleCase(r.Address)
	if r.City != "" {
		addr += ", " + titleCase(r.City)
	}
	if addr == "" {
		addr = "(no address on record)"
	}
	p.line(addr)
	stage := "mailed value, not yet certified"
	if r.AVStage == "certified" {
		stage = "certified value"
	}
	p.line("PIN %s · %s Township · class %s · tax year %d (%s)", search.FormatPIN(r.PIN), r.Township, r.Class, r.Year, stage)
	if r.Township != "" {
		p.line("")
		p.line("Board of Review: %s", windowText(r.Township, now))
	}
	if r.Unsupported != "" {
		p.line("")
		p.line("We can't compare this property: %s", r.Unsupported)
		p.line("")
		p.line(r.Disclaimer)
		return p.err
	}

	s, st := r.Subject, r.Stats
	p.line("")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "This home's building\t%s per sq ft\t(building AV %s on %s sq ft)\n", perSF(s.BldgAVPerSF), money(s.BldgAV), num(s.BldgSF))
	fmt.Fprintf(tw, "Assessed value\t%s total\t(building %s + land %s)\n", money(s.AV), money(s.BldgAV), money(s.LandAV))
	if st.NumComps > 0 {
		fmt.Fprintf(tw, "Similar homes' median\t%s per sq ft\t(building AV, %d similar homes)\n", perSF(st.MedianBldgAVPerSF), st.NumComps)
		fmt.Fprintf(tw, "Gap\t%s\t\n", gapPhrase(st.PctAboveMedian))
		fmt.Fprintf(tw, "Assessed lower\t%d of %d similar homes\t\n", countBelow(r), st.NumComps)
		fmt.Fprintf(tw, "At the median rate\t%s assessed value\t(median × this home's sq ft + its land AV %s", money(st.UniformityAV), money(s.LandAV))
		if st.PotentialAVReduction >= 1 {
			fmt.Fprintf(tw, "; %s below this home's", money(st.PotentialAVReduction))
		}
		fmt.Fprintln(tw, ")")
	}
	tw.Flush()
	p.line("%s", comps.LandNote)
	if r.Search != nil {
		p.line("Compared on: %s.", r.Search.Description)
	}
	if st.NumComps < 3 {
		p.line("Too few similar homes were found for a meaningful comparison.")
	}

	if len(r.Evidence) > 0 {
		p.line("")
		p.line("Similar homes with the building assessed lower per sq ft (most similar first):")
		homeTable(w, r.Evidence)
		p.line("")
		p.line("Differences from this home:")
		for i, c := range r.Evidence {
			p.line("  %d. %s", i+1, c.Differences)
		}
	}
	p.nbhd(r)
	if len(r.Flags) > 0 {
		p.line("")
		p.line("Worth double-checking:")
		for _, f := range r.Flags {
			p.line("  - %s", f.Message)
		}
	}
	p.line("")
	p.line(r.Disclaimer)
	return p.err
}

func (p *printer) nbhd(r *comps.Report) {
	p.line("")
	p.line("Board of Review outcomes in Assessor neighborhood %s:", r.Nbhd)
	if len(r.NbhdAppeals) == 0 {
		p.line("  No recent records.")
		return
	}
	for _, n := range r.NbhdAppeals {
		who := "Filed by attorneys or firms"
		if n.ProSe {
			who = "Filed by homeowners"
		}
		line := fmt.Sprintf("  %s: %s got a reduction (%d appeals, %d–%d)", who, pct(n.WinRate), n.Appeals, n.FirstYear, n.LastYear)
		if n.Decreases > 0 {
			line += fmt.Sprintf("; average cut %s of assessed value", pct(n.AvgCutPct))
		}
		p.line("%s", line)
	}
	p.line("  These are outcomes for people who chose to appeal. They aren't a prediction for any one home.")
}

func homeTable(w io.Writer, hs []comps.Comp) {
	rows := [][]string{{"#", "PIN", "Address", "Class", "Away", "Sq ft", "Built", "Beds/baths",
		"Building AV", "Land AV", "Total AV", "Bldg per sq ft"}}
	for i, c := range hs {
		rows = append(rows, []string{fmt.Sprint(i + 1), search.FormatPIN(c.PIN), titleCase(c.Address), c.Class,
			milesText(c.Miles), num(c.BldgSF), yearOr(c.YearBuilt), fmt.Sprintf("%d / %s", c.Beds, baths(c.Home)),
			money(c.BldgAV), money(c.LandAV), money(c.AV), perSF(c.BldgAVPerSF)})
	}
	table(w, "  ", rows, 4) // columns from index 4 on are numbers
}

// table prints rows in aligned columns: text columns left-aligned, columns
// from index numFrom on right-aligned.
func table(w io.Writer, indent string, rows [][]string, numFrom int) {
	var widths []int
	for _, r := range rows {
		for i, c := range r {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], utf8.RuneCountInString(c))
		}
	}
	for _, r := range rows {
		var b strings.Builder
		b.WriteString(indent)
		for i, c := range r {
			pad := strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c))
			if i > 0 {
				b.WriteString("  ")
			}
			if i >= numFrom {
				b.WriteString(pad + c)
			} else if i < len(r)-1 {
				b.WriteString(c + pad)
			} else {
				b.WriteString(c)
			}
		}
		fmt.Fprintln(w, b.String())
	}
}

func countBelow(r *comps.Report) int {
	n := 0
	for _, c := range r.Comps {
		if c.BldgAVPerSF < r.Subject.BldgAVPerSF { // building rate, as compared
			n++
		}
	}
	return n
}

type printer struct {
	w   io.Writer
	err error
}

func (p *printer) line(format string, a ...any) {
	if p.err != nil {
		return
	}
	if len(a) == 0 {
		_, p.err = fmt.Fprintln(p.w, format)
		return
	}
	_, p.err = fmt.Fprintf(p.w, format+"\n", a...)
}
