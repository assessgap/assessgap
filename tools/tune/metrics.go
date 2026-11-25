package main

import (
	"math"
	"sort"
)

// obs is one appeal in a metric: its score, label and bootstrap weight.
type obs struct {
	s float64
	y bool
	w float64
}

// auc is the weighted Mann-Whitney AUC: the chance a random winner scores
// higher than a random loser, ties counting half. With every weight 1 it is
// the backtest's own AUC (internal/cli/eval.go).
func auc(xs []obs) float64 {
	s := append([]obs(nil), xs...)
	sort.Slice(s, func(i, j int) bool { return s[i].s < s[j].s })
	return aucSorted(s)
}

// aucSorted needs xs sorted by score; it is O(n), for the bootstrap.
func aucSorted(s []obs) float64 {
	var lossBelow, num, wins, losses float64
	for i := 0; i < len(s); {
		j := i
		var tw, tl float64
		for j < len(s) && s[j].s == s[i].s {
			if s[j].y {
				tw += s[j].w
			} else {
				tl += s[j].w
			}
			j++
		}
		num += tw * (lossBelow + tl/2)
		lossBelow += tl
		wins += tw
		losses += tl
		i = j
	}
	if wins == 0 || losses == 0 {
		return 0.5
	}
	return num / (wins * losses)
}

// topShareWin is the win rate among the top share of appeals by score, and
// the share actually taken (ties at the cut are all included).
func topShareWin(xs []obs, share float64) (rate, taken float64) {
	s := append([]obs(nil), xs...)
	sort.Slice(s, func(i, j int) bool { return s[i].s > s[j].s })
	k := int(math.Ceil(share * float64(len(s))))
	if k == 0 {
		return 0, 0
	}
	for k < len(s) && s[k].s == s[k-1].s {
		k++
	}
	w := 0
	for _, o := range s[:k] {
		if o.y {
			w++
		}
	}
	return float64(w) / float64(k), float64(k) / float64(len(s))
}

// spearman is the rank correlation of a and b (ties take the average rank).
func spearman(a, b []float64) float64 {
	ra, rb := ranks(a), ranks(b)
	ma, mb := mean(ra), mean(rb)
	var sab, saa, sbb float64
	for i := range ra {
		da, db := ra[i]-ma, rb[i]-mb
		sab += da * db
		saa += da * da
		sbb += db * db
	}
	if saa == 0 || sbb == 0 {
		return 0
	}
	return sab / math.Sqrt(saa*sbb)
}

func ranks(xs []float64) []float64 {
	ix := make([]int, len(xs))
	for i := range ix {
		ix[i] = i
	}
	sort.Slice(ix, func(a, b int) bool { return xs[ix[a]] < xs[ix[b]] })
	r := make([]float64, len(xs))
	for i := 0; i < len(ix); {
		j := i
		for j < len(ix) && xs[ix[j]] == xs[ix[i]] {
			j++
		}
		for k := i; k < j; k++ {
			r[ix[k]] = float64(i+j+1) / 2
		}
		i = j
	}
	return r
}

// The report's tiers by gap, as internal/cli/eval.go tierBucket (3+ comps).
var tierNames = []string{">=+5%", "+1..+5%", "in line", "< -1%"}

func tierOf(gap float64) int {
	switch {
	case gap >= 0.05:
		return 0
	case gap >= 0.01:
		return 1
	case gap > -0.01:
		return 2
	default:
		return 3
	}
}

// tierTable holds each tier's count and wins.
type tierTable [4]struct{ n, wins int }

func (t tierTable) total() int { return t[0].n + t[1].n + t[2].n + t[3].n }

func (t tierTable) rate(i int) float64 {
	if t[i].n == 0 {
		return math.NaN()
	}
	return float64(t[i].wins) / float64(t[i].n)
}

func (t tierTable) share(i int) float64 { return float64(t[i].n) / float64(t.total()) }

// calibErr is how far the tier win rates of ref miss those observed in got,
// weighted by got's tier shares: the error of quoting ref's rates for got.
func calibErr(ref, got tierTable) float64 {
	e := 0.0
	for i := range got {
		if got[i].n > 0 && ref[i].n > 0 {
			e += got.share(i) * math.Abs(ref.rate(i)-got.rate(i))
		}
	}
	return e
}
