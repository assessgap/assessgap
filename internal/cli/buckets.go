package cli

import (
	"sort"

	"github.com/assessgap/assessgap/internal/comps"
)

// How the backtest groups and scores appeals. The bucket labels and cut
// points are frozen so runs stay comparable across versions; they don't follow
// the engine's thresholds (TestBuckets pins them at their boundaries).

// scored is one appeal's gap and outcome, for the separation measures.
type scored struct {
	gap float64
	won bool
}

// auc is the probability that a randomly chosen winner's gap is higher than
// a randomly chosen loser's (ties count half): the Mann-Whitney U statistic
// scaled to 0–1. 0.5 means the gap doesn't separate winners from losers.
func auc(xs []scored) float64 {
	s := append([]scored(nil), xs...)
	sort.Slice(s, func(i, j int) bool { return s[i].gap < s[j].gap })
	var rankSumWins float64
	wins := 0
	for i := 0; i < len(s); {
		j := i
		for j < len(s) && s[j].gap == s[i].gap {
			j++
		}
		rank := float64(i+j+1) / 2 // average 1-based rank of the tied run
		for k := i; k < j; k++ {
			if s[k].won {
				rankSumWins += rank
				wins++
			}
		}
		i = j
	}
	losses := len(s) - wins
	if wins == 0 || losses == 0 {
		return 0.5
	}
	return (rankSumWins - float64(wins)*float64(wins+1)/2) / (float64(wins) * float64(losses))
}

// tierOrder and tierBucket group gaps by the cut points the reports used
// until 2026-11-26, kept so runs stay comparable across versions.
var tierOrder = []string{"<3 comps", ">= +5%", "+1..+5%", "in line (±1%)", "< -1%"}

func tierBucket(gap float64, n int) string {
	switch {
	case n < 3:
		return "<3 comps"
	case gap >= 0.05:
		return ">= +5%"
	case gap >= 0.01:
		return "+1..+5%"
	case gap > -0.01:
		return "in line (±1%)"
	default:
		return "< -1%"
	}
}

var groupOrder = []string{"<3 comps", ">= +1%, >= 3/4 lower", ">= +1%, < 3/4 lower", "in line (±1%)", "< -1%, >= 1/4 lower", "< -1%, < 1/4 lower"}

func groupBucket(st comps.Stats) string {
	if st.NumComps < 3 {
		return "<3 comps"
	}
	return groupOrder[1+comps.Group(st)]
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
