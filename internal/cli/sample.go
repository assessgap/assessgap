package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/assessgap/assessgap/internal/comps"
)

// assessgap sample: coverage and the comparison mix on a random sample of
// homes in the townships whose Board of Review dates aren't posted yet.

// In-scope homes in the given townships, by current parcel class.
func samplePINsSQL(townships int) string {
	return `
SELECT pin FROM parcel
WHERE township_name IN (` + placeholders(townships) + `)
  AND class IN (` + placeholders(len(comps.ScopeClasses())) + `)
ORDER BY pin`
}

func evalSample(ctx context.Context, w io.Writer, logf func(string, ...any), db *sql.DB, e *comps.Engine, n int, seed uint64) error {
	towns := unscheduledTownships(time.Now())
	pins, err := queryStrings(ctx, db, samplePINsSQL(len(towns)), append(anys(towns), anys(comps.ScopeClasses())...)...)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "in-scope PINs in the %d townships without posted BOR dates: %d\n", len(towns), len(pins))
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
				logf("%s: %v", pin, err)
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
	fmt.Fprintf(w, "sampled %d PINs in %s (seed %d)\n", total, time.Since(start).Round(time.Second), seed)
	fmt.Fprintf(w, "no assessment row: %d\n", errs)
	fmt.Fprintf(w, "report years: %v   AV stage: %v\n", years, stages)
	fmt.Fprintf(w, "unsupported: %d\n", sum(unsupported))
	for k, v := range unsupported {
		fmt.Fprintf(w, "  %5d  %s\n", v, k)
	}
	fmt.Fprintf(w, "supported: %d\n", supported)
	fmt.Fprintf(w, "  >= 8 comps: %d (%.1f%% of supported, %.1f%% of sampled)\n", ge8, pctOf(ge8, supported), pctOf(ge8, total))
	fmt.Fprintln(w, "verdicts (of supported):")
	for _, v := range verdicts {
		fmt.Fprintf(w, "  %-8s %5d  %5.1f%%\n", v, byVerdict[v], pctOf(byVerdict[v], supported))
	}
	fmt.Fprintln(w, "search level used:")
	for l := 0; l <= 3; l++ {
		fmt.Fprintf(w, "  L%d %5d  %5.1f%%\n", l, byLevel[l], pctOf(byLevel[l], supported))
	}
	fmt.Fprintln(w, "please-verify flags (count of reports with flag):")
	for k, v := range flags {
		fmt.Fprintf(w, "  %-8s %5d  %5.1f%%\n", k, v, pctOf(v, supported))
	}
	fmt.Fprintln(w, "verdicts by township:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "township\tn\tstrong\tpossible\tweak\t")
	for _, t := range sortedKeys(byTownship) {
		m := byTownship[t]
		n := m[comps.Strong] + m[comps.Possible] + m[comps.Weak]
		fmt.Fprintf(tw, "%s\t%d\t%.0f%%\t%.0f%%\t%.0f%%\t\n", t, n,
			pctOf(m[comps.Strong], n), pctOf(m[comps.Possible], n), pctOf(m[comps.Weak], n))
	}
	tw.Flush()
	fmt.Fprintln(w, "example PINs:")
	for _, k := range sortedKeys(examples) {
		fmt.Fprintf(w, "  %-12s %s\n", k, strings.Join(examples[k], " "))
	}
	return nil
}

func addExample(m map[string][]string, k, pin string) {
	if len(m[k]) < 5 {
		m[k] = append(m[k], pin)
	}
}
