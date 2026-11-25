// Command compseval measures the comps engine on the full database.
//
//	compseval [-db path] sample   [-n 2000] [-seed 1]   coverage + verdict mix in the 16 upcoming BOR townships
//	compseval [-db path] backtest [-years 2024,2025]    verdict vs. actual BOR outcome
//
// Output is plain text for pasting into docs.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/store"
)

// Townships the BOR is expected to open Dec 2026 – Feb 2027 (business plan §2).
// The data names the Chicago ones "North Chicago" / "South Chicago".
var upcomingTownships = []string{
	"Lake", "Jefferson", "North Chicago", "South Chicago", "Worth", "Proviso", "Leyden", "Wheeling",
	"Palatine", "Orland", "Schaumburg", "Thornton", "Niles", "Hanover", "Rich", "Bloom",
}

var verdicts = []string{comps.Strong, comps.Possible, comps.Weak}

func main() {
	log.SetFlags(0)
	dbPath := flag.String("db", "data/cook.db", "database built by cmd/build")
	n := flag.Int("n", 2000, "sample size (sample mode)")
	seed := flag.Uint64("seed", 1, "random seed (sample mode)")
	years := flag.String("years", "2024,2025", "BOR tax years (backtest mode)")
	flag.Parse()

	if _, err := os.Stat(*dbPath); err != nil {
		log.Fatal(err)
	}
	db, err := store.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	e := comps.New(db)
	e.CachePools = true
	ctx := context.Background()

	switch flag.Arg(0) {
	case "sample":
		err = runSample(ctx, db, e, *n, *seed)
	case "backtest":
		err = runBacktest(ctx, db, e, *years)
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func placeholders(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

func anys[T any](xs []T) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

func queryStrings(ctx context.Context, db *sql.DB, q string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- sample

// In-scope homes in the upcoming townships, by current parcel class.
var samplePINsSQL = `
SELECT pin FROM parcel
WHERE township_name IN (` + placeholders(len(upcomingTownships)) + `)
  AND class IN (` + placeholders(len(comps.ScopeClasses())) + `)
ORDER BY pin`

func runSample(ctx context.Context, db *sql.DB, e *comps.Engine, n int, seed uint64) error {
	pins, err := queryStrings(ctx, db, samplePINsSQL, append(anys(upcomingTownships), anys(comps.ScopeClasses())...)...)
	if err != nil {
		return err
	}
	fmt.Printf("in-scope PINs in the 16 townships: %d\n", len(pins))
	rng := rand.New(rand.NewPCG(seed, seed))
	rng.Shuffle(len(pins), func(i, j int) { pins[i], pins[j] = pins[j], pins[i] })
	if len(pins) > n {
		pins = pins[:n]
	}

	var (
		total, errs, ge8 int
		years            = map[int]int{}
		stages           = map[string]int{}
		unsupported      = map[string]int{}
		byVerdict        = map[string]int{}
		byLevel          = map[int]int{}
		byTownship       = map[string]map[string]int{}
		flags            = map[string]int{}
		examples         = map[string][]string{}
	)
	start := time.Now()
	for _, pin := range pins {
		r, err := e.Report(ctx, pin, 0)
		total++
		if err != nil {
			errs++
			if !errors.Is(err, comps.ErrNotFound) {
				log.Printf("%s: %v", pin, err)
			}
			continue
		}
		years[r.Year]++
		stages[r.AVStage]++
		if r.Unsupported != "" {
			unsupported[firstSentence(r.Unsupported)]++
			addExample(examples, "unsupported", pin)
			continue
		}
		if r.Stats.NumComps >= 8 {
			ge8++
		}
		byVerdict[r.Verdict]++
		byLevel[r.Search.Level]++
		if byTownship[r.Township] == nil {
			byTownship[r.Township] = map[string]int{}
		}
		byTownship[r.Township][r.Verdict]++
		for _, f := range r.Flags {
			flags[f.Field]++
		}
		addExample(examples, r.Verdict, pin)
		if r.Search.Level >= 2 {
			addExample(examples, fmt.Sprintf("widened-L%d", r.Search.Level), pin)
		}
		if r.Stats.NumComps < 8 {
			addExample(examples, "lt8comps", pin)
		}
		if len(r.Flags) > 0 {
			addExample(examples, "flagged", pin)
		}
	}
	supported := total - errs - sum(unsupported)
	fmt.Printf("sampled %d PINs in %s (seed %d)\n", total, time.Since(start).Round(time.Second), seed)
	fmt.Printf("no assessment row: %d\n", errs)
	fmt.Printf("report years: %v   AV stage: %v\n", years, stages)
	fmt.Printf("unsupported: %d\n", sum(unsupported))
	for k, v := range unsupported {
		fmt.Printf("  %5d  %s\n", v, k)
	}
	fmt.Printf("supported: %d\n", supported)
	fmt.Printf("  >= 8 comps: %d (%.1f%% of supported, %.1f%% of sampled)\n", ge8, pctOf(ge8, supported), pctOf(ge8, total))
	fmt.Println("verdicts (of supported):")
	for _, v := range verdicts {
		fmt.Printf("  %-8s %5d  %5.1f%%\n", v, byVerdict[v], pctOf(byVerdict[v], supported))
	}
	fmt.Println("search level used:")
	for l := 0; l <= 3; l++ {
		fmt.Printf("  L%d %5d  %5.1f%%\n", l, byLevel[l], pctOf(byLevel[l], supported))
	}
	fmt.Println("please-verify flags (count of reports with flag):")
	for k, v := range flags {
		fmt.Printf("  %-8s %5d  %5.1f%%\n", k, v, pctOf(v, supported))
	}
	fmt.Println("verdicts by township:")
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "township\tn\tstrong\tpossible\tweak\t")
	for _, t := range sortedKeys(byTownship) {
		m := byTownship[t]
		n := m[comps.Strong] + m[comps.Possible] + m[comps.Weak]
		fmt.Fprintf(tw, "%s\t%d\t%.0f%%\t%.0f%%\t%.0f%%\t\n", t, n,
			pctOf(m[comps.Strong], n), pctOf(m[comps.Possible], n), pctOf(m[comps.Weak], n))
	}
	tw.Flush()
	fmt.Println("example PINs:")
	for _, k := range sortedKeys(examples) {
		fmt.Printf("  %-12s %s\n", k, strings.Join(examples[k], " "))
	}
	return nil
}

func addExample(m map[string][]string, k, pin string) {
	if len(m[k]) < 5 {
		m[k] = append(m[k], pin)
	}
}

// ---------------------------------------------------------------- backtest

// One row per PIN and tax year: residential home appeals with a clear result.
// If a PIN has several appeals in a year (rare), any Decrease counts as a win,
// and it counts as pro se if any filing was.
var backtestSQL = `
SELECT pin, tax_year,
       MAX(result = 'Decrease')                        AS won,
       MAX(pro_se)                                     AS pro_se,
       MAX(CASE WHEN result = 'Decrease' AND av_before > 0
                THEN (av_before - av_after) / av_before END) AS cut_pct
FROM bor_appeal
WHERE tax_year IN (%s)
  AND substr(class, 1, 3) IN (` + placeholders(len(comps.ScopeClasses())) + `)
  AND result IN ('Decrease', 'No Change')
GROUP BY pin, tax_year
ORDER BY tax_year, pin`

type outcome struct {
	n, wins int
	cutSum  float64
}

func (o *outcome) add(won bool, cut float64) {
	o.n++
	if won {
		o.wins++
		o.cutSum += cut
	}
}

func (o outcome) String() string {
	if o.n == 0 {
		return "0 | - | -"
	}
	avgCut := "-"
	if o.wins > 0 {
		avgCut = fmt.Sprintf("%.1f%%", 100*o.cutSum/float64(o.wins))
	}
	return fmt.Sprintf("%d | %.1f%% | %s", o.n, pctOf(o.wins, o.n), avgCut)
}

func runBacktest(ctx context.Context, db *sql.DB, e *comps.Engine, yearList string) error {
	ys := strings.Split(yearList, ",")
	q := fmt.Sprintf(backtestSQL, placeholders(len(ys)))
	rows, err := db.QueryContext(ctx, q, append(anys(ys), anys(comps.ScopeClasses())...)...)
	if err != nil {
		return err
	}
	type appeal struct {
		pin      string
		year     int
		won, pro bool
		cut      sql.NullFloat64
	}
	var appeals []appeal
	for rows.Next() {
		var a appeal
		if err := rows.Scan(&a.pin, &a.year, &a.won, &a.pro, &a.cut); err != nil {
			rows.Close()
			return err
		}
		appeals = append(appeals, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	fmt.Printf("in-scope BOR appeals (PIN-years) %s: %d\n", yearList, len(appeals))

	type key struct {
		year    int
		verdict string
		proSe   bool
	}
	var (
		byKey        = map[key]*outcome{}
		byGap        = map[string]map[bool]*outcome{}
		skipped      = map[string]int{}
		reportErrors int
	)
	get := func(k key) *outcome {
		if byKey[k] == nil {
			byKey[k] = &outcome{}
		}
		return byKey[k]
	}
	start := time.Now()
	for i, a := range appeals {
		if i > 0 && i%20000 == 0 {
			log.Printf("  %d/%d (%s)", i, len(appeals), time.Since(start).Round(time.Second))
		}
		r, err := e.Report(ctx, a.pin, a.year)
		if err != nil {
			reportErrors++
			continue
		}
		if r.Unsupported != "" {
			skipped[firstSentence(r.Unsupported)]++
			continue
		}
		v := r.Verdict
		for _, yr := range []int{a.year, 0} { // 0 = all years
			get(key{yr, v, a.pro}).add(a.won, a.cut.Float64)
		}
		g := gapBucket(r.Stats.PctAboveMedian, r.Stats.NumComps)
		if byGap[g] == nil {
			byGap[g] = map[bool]*outcome{false: {}, true: {}}
		}
		byGap[g][a.pro].add(a.won, a.cut.Float64)
	}
	fmt.Printf("scored in %s; errors %d; skipped (unsupported) %d\n",
		time.Since(start).Round(time.Second), reportErrors, sum(skipped))
	for k, v := range skipped {
		fmt.Printf("  %6d  %s\n", v, k)
	}

	for _, p := range append([]string{"all"}, ys...) {
		yr := 0
		if p != "all" {
			fmt.Sscan(p, &yr)
		}
		fmt.Printf("\n**Tax year %s**\n\n", p)
		printOutcomeHeader("verdict")
		for _, v := range verdicts {
			printOutcomeRow(v, deref(byKey[key{yr, v, true}]), deref(byKey[key{yr, v, false}]))
		}
	}
	fmt.Printf("\n**By subject AV/sf vs. comp median (all years)**\n\n")
	printOutcomeHeader("gap")
	for _, g := range gapOrder {
		if m := byGap[g]; m != nil {
			printOutcomeRow(g, *m[true], *m[false])
		}
	}
	return nil
}

func printOutcomeHeader(first string) {
	fmt.Printf("| %s | pro se n | win | avg cut | attorney n | win | avg cut | all n | win | avg cut |\n", first)
	fmt.Println("|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|")
}

func printOutcomeRow(label string, pro, att outcome) {
	all := outcome{n: pro.n + att.n, wins: pro.wins + att.wins, cutSum: pro.cutSum + att.cutSum}
	fmt.Printf("| %s | %s | %s | %s |\n", label, pro, att, all)
}

var gapOrder = []string{"<3 comps", "< -10%", "-10..-5%", "-5..0%", "0..5%", "5..10%", "10..20%", "20..30%", ">= 30%"}

func gapBucket(gap float64, n int) string {
	switch {
	case n < 3:
		return "<3 comps"
	case gap < -0.10:
		return "< -10%"
	case gap < -0.05:
		return "-10..-5%"
	case gap < 0:
		return "-5..0%"
	case gap < 0.05:
		return "0..5%"
	case gap < 0.10:
		return "5..10%"
	case gap < 0.20:
		return "10..20%"
	case gap < 0.30:
		return "20..30%"
	default:
		return ">= 30%"
	}
}

func deref(o *outcome) outcome {
	if o == nil {
		return outcome{}
	}
	return *o
}

// ---------------------------------------------------------------- helpers

func firstSentence(s string) string {
	if i := strings.Index(s, ";"); i > 0 {
		return s[:i]
	}
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i]
	}
	return s
}

func sum[K comparable](m map[K]int) int {
	t := 0
	for _, v := range m {
		t += v
	}
	return t
}

func pctOf(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
