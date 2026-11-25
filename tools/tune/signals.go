package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"time"
)

// Context signals: facts on record before the Board decides, outside the
// comparison itself. Each is measured by how much it adds to a small logistic
// model of the gap alone, cross-validated by township and across years.

// gapFeatures is a piecewise-linear spline of the current gap (knots at −5%,
// 0, +5%, +20%), so the base model can follow the plateau above +5%.
func gapFeatures(g float64) []float64 {
	g = math.Max(-0.5, math.Min(1, g))
	return []float64{g, math.Max(0, g+0.05), math.Max(0, g), math.Max(0, g-0.05), math.Max(0, g-0.2)}
}

// reassessed reports whether a triad was reassessed in a tax year. From the
// data (mailed vs prior final values): South 2023, City 2024, North 2025,
// every three years; the Assessor publishes the schedule in advance.
func reassessed(triad string, year int) bool {
	base := map[string]int{"1": 2024, "2": 2025, "3": 2023}[triad]
	return base != 0 && (year-base)%3 == 0
}

func priorChange(a Appeal) (float64, bool) {
	if a.PriorFinal <= 0 || a.Certified <= 0 {
		return 0, false
	}
	return math.Max(-0.5, math.Min(1.5, a.Certified/a.PriorFinal-1)), true
}

func assessorCut(a Appeal) float64 {
	if a.Mailed <= 0 || a.Certified <= 0 {
		return 0
	}
	return math.Max(-0.5, math.Min(0.5, 1-a.Certified/a.Mailed))
}

// nbhdPrior is the global pro se win rate the neighborhood rates shrink to.
type nbhdPrior struct{ rate float64 }

const shrinkK = 20

func (np nbhdPrior) logit(a Appeal) float64 {
	r := (float64(a.NbhdWins) + shrinkK*np.rate) / (float64(a.NbhdN) + shrinkK)
	r = math.Max(0.01, math.Min(0.99, r))
	return math.Log(r / (1 - r))
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// signal adds features for one appeal.
type signal struct {
	name string
	feat func(a Appeal, s Scored, np nbhdPrior) []float64
}

var signals = []signal{
	{"prior_change", func(a Appeal, _ Scored, _ nbhdPrior) []float64 {
		dp, ok := priorChange(a)
		return []float64{dp, math.Max(0, dp-0.1), math.Max(0, dp-0.2), b2f(!ok)}
	}},
	{"assessor_cut", func(a Appeal, _ Scored, _ nbhdPrior) []float64 {
		c := assessorCut(a)
		return []float64{c, b2f(c > 0.001)}
	}},
	{"prior_appeals", func(a Appeal, _ Scored, _ nbhdPrior) []float64 {
		return []float64{b2f(a.PriorWins > 0), b2f(a.PriorAppeals > 0 && a.PriorWins == 0)}
	}},
	{"nbhd_rate", func(a Appeal, _ Scored, np nbhdPrior) []float64 { return []float64{np.logit(a)} }},
	{"reassessment", func(a Appeal, _ Scored, _ nbhdPrior) []float64 { return []float64{b2f(reassessed(a.Triad, a.Year))} }},
	{"data_flags", func(_ Appeal, s Scored, _ nbhdPrior) []float64 {
		return []float64{b2f(s.Flags&^16 != 0), b2f(s.Flags&16 != 0)}
	}},
	{"search_widened", func(_ Appeal, s Scored, _ nbhdPrior) []float64 {
		return []float64{b2f(s.Level >= 1), b2f(s.Level >= 2), b2f(s.N < 8)}
	}},
}

// modelFeatures builds a row for the named model: "gap" alone, "gap+<signal>",
// or "all" (the gap and every signal).
func modelFeatures(model string, a Appeal, s Scored, np nbhdPrior) []float64 {
	x := gapFeatures(s.Gap)
	for _, sg := range signals {
		if model == "all" || model == "gap+"+sg.name {
			x = append(x, sg.feat(a, s, np)...)
		}
	}
	return x
}

func globalPrior(d *Dataset) nbhdPrior {
	type ky struct {
		nb string
		y  int
	}
	seen := map[ky]bool{}
	var n, w int
	for _, a := range d.Appeals {
		k := ky{a.Nbhd, a.Year}
		if !seen[k] {
			seen[k] = true
			n += a.NbhdN
			w += a.NbhdWins
		}
	}
	if n == 0 {
		return nbhdPrior{0.5}
	}
	return nbhdPrior{float64(w) / float64(n)}
}

// ------------------------------------------------------------ logistic model

type logit struct {
	mu, sd, beta []float64 // beta[0] is the intercept
}

// fitLogit fits by Newton's method on standardized features, with a small
// ridge penalty so collinear spline terms stay well defined.
func fitLogit(X [][]float64, y []bool) logit {
	p := len(X[0])
	m := logit{mu: make([]float64, p), sd: make([]float64, p), beta: make([]float64, p+1)}
	for j := 0; j < p; j++ {
		var s, ss float64
		for _, x := range X {
			s += x[j]
			ss += x[j] * x[j]
		}
		n := float64(len(X))
		m.mu[j] = s / n
		m.sd[j] = math.Sqrt(math.Max(ss/n-m.mu[j]*m.mu[j], 0))
		if m.sd[j] < 1e-9 {
			m.sd[j] = 1
		}
	}
	z := make([]float64, p+1)
	const lambda = 1.0
	for iter := 0; iter < 25; iter++ {
		H := make([][]float64, p+1)
		for i := range H {
			H[i] = make([]float64, p+1)
		}
		g := make([]float64, p+1)
		for i, x := range X {
			m.row(x, z)
			eta := dot(m.beta, z)
			pr := 1 / (1 + math.Exp(-eta))
			r := b2f(y[i]) - pr
			wt := pr * (1 - pr)
			for a := 0; a <= p; a++ {
				g[a] += r * z[a]
				for b := 0; b <= a; b++ {
					H[a][b] += wt * z[a] * z[b]
				}
			}
		}
		for a := 0; a <= p; a++ {
			for b := 0; b < a; b++ {
				H[b][a] = H[a][b]
			}
			if a > 0 {
				H[a][a] += lambda
				g[a] -= lambda * m.beta[a]
			}
		}
		step := solve(H, g)
		maxStep := 0.0
		for a := range step {
			m.beta[a] += step[a]
			maxStep = math.Max(maxStep, math.Abs(step[a]))
		}
		if maxStep < 1e-7 {
			break
		}
	}
	return m
}

func (m logit) row(x, z []float64) {
	z[0] = 1
	for j := range x {
		z[j+1] = (x[j] - m.mu[j]) / m.sd[j]
	}
}

func (m logit) predict(x []float64) float64 {
	z := make([]float64, len(x)+1)
	m.row(x, z)
	return 1 / (1 + math.Exp(-dot(m.beta, z)))
}

func dot(a, b []float64) float64 {
	s := 0.0
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

// solve does Gaussian elimination with partial pivoting on a copy.
func solve(A [][]float64, b []float64) []float64 {
	n := len(b)
	M := make([][]float64, n)
	for i := range M {
		M[i] = append(append([]float64(nil), A[i]...), b[i])
	}
	for c := 0; c < n; c++ {
		piv := c
		for r := c + 1; r < n; r++ {
			if math.Abs(M[r][c]) > math.Abs(M[piv][c]) {
				piv = r
			}
		}
		M[c], M[piv] = M[piv], M[c]
		if math.Abs(M[c][c]) < 1e-12 {
			continue
		}
		for r := c + 1; r < n; r++ {
			f := M[r][c] / M[c][c]
			for k := c; k <= n; k++ {
				M[r][k] -= f * M[c][k]
			}
		}
	}
	x := make([]float64, n)
	for r := n - 1; r >= 0; r-- {
		if math.Abs(M[r][r]) < 1e-12 {
			continue
		}
		s := M[r][n]
		for k := r + 1; k < n; k++ {
			s -= M[r][k] * x[k]
		}
		x[r] = s / M[r][r]
	}
	return x
}

// ------------------------------------------------------------ signals command

// cvResult is a model's out-of-fold AUC (township folds) and its
// train-2023 → test-2024 AUC, on pro se appeals.
type cvResult struct {
	cv, yearSplit float64
	foldAUC       [nFolds]float64
}

func crossValidate(d *Dataset, base []Scored, pop []int, model string, np nbhdPrior) cvResult {
	fold := folds(d)
	var res cvResult
	X := make([][]float64, len(pop))
	for k, i := range pop {
		X[k] = modelFeatures(model, d.Appeals[i], base[i], np)
	}
	oof := make([]obs, len(pop))
	for f := 0; f < nFolds; f++ {
		var tx [][]float64
		var ty []bool
		for k, i := range pop {
			if fold[d.Appeals[i].Town] != f {
				tx = append(tx, X[k])
				ty = append(ty, d.Appeals[i].Won)
			}
		}
		m := fitLogit(tx, ty)
		var fo []obs
		for k, i := range pop {
			if fold[d.Appeals[i].Town] == f {
				oof[k] = obs{m.predict(X[k]), d.Appeals[i].Won, 1}
				fo = append(fo, oof[k])
			}
		}
		res.foldAUC[f] = auc(fo)
	}
	res.cv = auc(oof)
	var tx [][]float64
	var ty []bool
	for k, i := range pop {
		if d.Appeals[i].Year == 2023 {
			tx = append(tx, X[k])
			ty = append(ty, d.Appeals[i].Won)
		}
	}
	m := fitLogit(tx, ty)
	var te []obs
	for k, i := range pop {
		if d.Appeals[i].Year == 2024 {
			te = append(te, obs{m.predict(X[k]), d.Appeals[i].Won, 1})
		}
	}
	res.yearSplit = auc(te)
	return res
}

func proPopulation(d *Dataset, base []Scored) []int {
	var pop []int
	for _, i := range population(base) {
		if d.Appeals[i].Pro {
			pop = append(pop, i)
		}
	}
	return pop
}

func runSignals(cache, logPath string) error {
	d, err := loadTuning(cache)
	if err != nil {
		return err
	}
	start := time.Now()
	base, err := score(d, Trial{Name: "current"}, true)
	if err != nil {
		return err
	}
	pop := proPopulation(d, base)
	np := globalPrior(d)
	w := os.Stdout
	fmt.Fprintf(w, "pro se appeals, 2023–2024, current rule, 3+ comps: %d (scored in %s); neighborhood rates shrink to %.3f with k=%d\n\n",
		len(pop), time.Since(start).Round(time.Second), np.rate, shrinkK)

	models := []string{"gap"}
	for _, s := range signals {
		models = append(models, "gap+"+s.name)
	}
	models = append(models, "all")
	var gapRes cvResult
	fmt.Fprintln(w, "| model | CV AUC (township folds) | Δ vs gap | folds up | 2023→2024 AUC | Δ vs gap |")
	fmt.Fprintln(w, "|---|--:|--:|--:|--:|--:|")
	for _, model := range models {
		t0 := time.Now()
		r := crossValidate(d, base, pop, model, np)
		if model == "gap" {
			gapRes = r
		}
		up := 0
		for f := range r.foldAUC {
			if r.foldAUC[f] > gapRes.foldAUC[f] {
				up++
			}
		}
		fmt.Fprintf(w, "| %s | %.4f | %+.4f | %d/5 | %.4f | %+.4f |\n", model, r.cv, r.cv-gapRes.cv, up, r.yearSplit, r.yearSplit-gapRes.yearSplit)
		secs := time.Since(t0).Seconds()
		if err := appendLog(logPath, func(id int) []string {
			row := []string{strconv.Itoa(id), "signal", model, `{"model":"` + model + `","fit":"logistic, township 5-fold CV and 2023→2024"}`,
				strconv.Itoa(len(pop)), "-", f3(r.cv), "-", f3(r.yearSplit), f3(r.cv - gapRes.cv), "-", f3(r.yearSplit - gapRes.yearSplit),
				fmt.Sprintf("%d/5", up), "-", "-", "-", "-", "-", "-", "-", "-", "-"}
			return append(row, fmt.Sprintf("%.0f", secs))
		}); err != nil {
			return err
		}
	}
	fmt.Fprintln(w)
	describeSignals(w, d, base, pop)
	return nil
}

// describeSignals prints plain win rates, with shares, for each signal's
// groups, overall and within the report's gap tiers.
func describeSignals(w io.Writer, d *Dataset, base []Scored, pop []int) {
	groups := []struct {
		name string
		of   func(a Appeal, s Scored) string
	}{
		{"change from last year's final assessment", func(a Appeal, _ Scored) string {
			dp, ok := priorChange(a)
			switch {
			case !ok:
				return "no prior value"
			case dp < 0:
				return "fell"
			case dp < 0.10:
				return "rose 0–10%"
			case dp < 0.20:
				return "rose 10–20%"
			case dp < 0.40:
				return "rose 20–40%"
			default:
				return "rose 40%+"
			}
		}},
		{"Assessor-stage change this year", func(a Appeal, _ Scored) string {
			if assessorCut(a) > 0.001 {
				return "Assessor lowered it"
			}
			return "no Assessor reduction"
		}},
		{"Board appeals on this PIN in the 2 prior years", func(a Appeal, _ Scored) string {
			switch {
			case a.PriorWins > 0:
				return "got a reduction before"
			case a.PriorAppeals > 0:
				return "appealed, no reduction"
			default:
				return "no prior appeal"
			}
		}},
		{"reassessment year", func(a Appeal, _ Scored) string {
			if reassessed(a.Triad, a.Year) {
				return "reassessed this year"
			}
			return "not reassessed"
		}},
		{"data checks", func(_ Appeal, s Scored) string {
			switch {
			case s.Flags&16 != 0:
				return "land flag"
			case s.Flags != 0:
				return "other flag"
			default:
				return "no flag"
			}
		}},
		{"search widened", func(_ Appeal, s Scored) string { return "level " + strconv.Itoa(s.Level) }},
	}
	for _, g := range groups {
		type cell struct{ n, wins int }
		tot := map[string]*cell{}
		byTier := map[string]*[4]cell{}
		for _, i := range pop {
			a, s := d.Appeals[i], base[i]
			k := g.of(a, s)
			if tot[k] == nil {
				tot[k] = &cell{}
				byTier[k] = &[4]cell{}
			}
			t := tierOf(s.Gap)
			tot[k].n++
			byTier[k][t].n++
			if a.Won {
				tot[k].wins++
				byTier[k][t].wins++
			}
		}
		keys := make([]string, 0, len(tot))
		for k := range tot {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Fprintf(w, "**%s** (pro se, share of appeals / reduction rate; then by gap tier)\n\n", g.name)
		fmt.Fprintln(w, "| group | share | won | ≥+5% | +1..+5% | in line | < −1% |")
		fmt.Fprintln(w, "|---|--:|--:|--:|--:|--:|--:|")
		for _, k := range keys {
			c := tot[k]
			fmt.Fprintf(w, "| %s | %.1f%% (%d) | %.1f%% |", k, 100*float64(c.n)/float64(len(pop)), c.n, 100*float64(c.wins)/float64(c.n))
			for t := 0; t < 4; t++ {
				tc := byTier[k][t]
				if tc.n == 0 {
					fmt.Fprint(w, " - |")
					continue
				}
				fmt.Fprintf(w, " %.1f%% (%d) |", 100*float64(tc.wins)/float64(tc.n), tc.n)
			}
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w)
	}
}
