package comps

import (
	"fmt"
	"sort"
	"time"

	"github.com/dustin/go-humanize"
)

// The engine's thresholds. The surfaces use them through InLine,
// AboveMedian, Stats.TooFewToCompare and Stats.StatesReduction, never as
// literals. The quoted rates are groupRates, below.
const (
	// possibleGap: within ±1% of the median is "in line" on every surface; from
	// +1% the reports state the value at the median rate, and the evidence
	// report may be offered.
	possibleGap = 0.01
	// minComparable: with fewer similar homes, every surface says there are too
	// few to compare, and there is no evidence report.
	minComparable = 3
	// minEvidence and maxEvidence: how many lower-assessed homes a stronger
	// tier needs, and how many the reports list.
	minEvidence = 3
	maxEvidence = 6
)

// TooFewToCompare reports whether there are too few similar homes to state a
// median. Every surface then says so instead of comparing.
func (st Stats) TooFewToCompare() bool { return st.NumComps < minComparable }

// StatesReduction reports whether the reports state how much lower the value
// at the median rate is: 1% or more above the median, by at least $1. Below
// that, every surface states the value at the median rate without a
// difference.
func (st Stats) StatesReduction() bool {
	return AboveMedian(st.PctAboveMedian) && st.PotentialAVReduction >= 1
}

// Shares of similar homes whose building is assessed lower per sq ft than
// the subject's (strictly, so a home assessed the same as its neighbors gains
// nothing). A home's rank among its comps separated Board of Review outcomes
// better than its gap from the median (docs/dev/2026-11-26-tune.md).
const (
	strongShare = 0.75
	lowShare    = 0.25
)

// ShareLower is the share of comps assessed lower per sq ft than the subject.
func (st Stats) ShareLower() float64 {
	if st.NumComps == 0 {
		return 0
	}
	return float64(st.CompsLower) / float64(st.NumComps)
}

// Group places a comparison in one of the five groups the reports quote
// backtest rates for. The gap from the median comes first, because every
// surface states it, so a quoted rate never describes a home as above the
// median when the report says it is in line or below. Within the above- and
// below-median groups the share assessed lower splits further:
//
//	0: at least 1% above the median, higher than at least three quarters
//	1: at least 1% above the median, higher than fewer than three quarters
//	2: within 1% of the median
//	3: more than 1% below the median, higher than at least a quarter
//	4: more than 1% below the median, higher than fewer than a quarter
func Group(st Stats) int {
	gap, share := st.PctAboveMedian, st.ShareLower()
	switch {
	case AboveMedian(gap) && share >= strongShare:
		return 0
	case AboveMedian(gap):
		return 1
	case InLine(gap):
		return 2
	case share >= lowShare:
		return 3
	default:
		return 4
	}
}

// RankScore orders homes by their percentile among their comps, the gap
// breaking ties (it moves the score by under one percentile step). It is the
// score the 2026-11-26 tuning validated, for the backtest's AUC.
func RankScore(st Stats) float64 {
	return st.SubjectPercentile + 0.0005*max(-1, min(1, st.PctAboveMedian))
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// computeStats compares the subject's building AV per sq ft with the comps'.
// The Assessor values land separately at a neighborhood rate per land sq ft,
// so the uniformity figure applies the comps' median building rate to the
// subject's building and adds back the subject's own land AV.
func computeStats(subject Home, comps []Comp) Stats {
	vals := make([]float64, len(comps))
	var land []float64
	below, equal := 0, 0
	for i, c := range comps {
		vals[i] = c.BldgAVPerSF
		switch {
		case c.BldgAVPerSF < subject.BldgAVPerSF:
			below++
		case c.BldgAVPerSF == subject.BldgAVPerSF:
			equal++
		}
		if r := c.LandAVPerSF(); r > 0 {
			land = append(land, r)
		}
	}
	st := Stats{NumComps: len(comps), SubjectBldgAVPerSF: round(subject.BldgAVPerSF, 2), CompsLower: below}
	if r := subject.LandAVPerSF(); r > 0 && len(land) > 0 {
		st.SubjectLandAVPerSF, st.MedianLandAVPerSF = round(r, 2), round(median(land), 2)
	}
	if len(comps) == 0 {
		return st
	}
	med := median(vals)
	st.MedianBldgAVPerSF = round(med, 2)
	st.PctAboveMedian = round(subject.BldgAVPerSF/med-1, 4)
	st.SubjectPercentile = round(100*(float64(below)+0.5*float64(equal))/float64(len(comps)), 1)
	// From the median as shown (to the cent), so the figure is exactly the sum
	// the report prints: median × sq ft + land AV.
	st.UniformityAV = round(st.MedianBldgAVPerSF*subject.BldgSF+subject.LandAV, 0)
	st.PotentialAVReduction = max(0, round(subject.AV-st.UniformityAV, 0))
	return st
}

// ComputeStats is computeStats for callers outside the package (tools/tune).
func ComputeStats(subject Home, comps []Comp) Stats { return computeStats(subject, comps) }

// DataFlags returns the "worth double-checking" items a report would show.
func DataFlags(subject Home, pool []Home, comps []Comp) []Flag {
	return append(characteristicFlags(subject, pool), landFlag(subject, comps, computeStats(subject, comps))...)
}

// evidenceComps picks the most similar comps whose building is assessed lower
// per sqft than the subject's: the ones a homeowner would cite. comps must be
// sorted nearest first.
func evidenceComps(subject Home, comps []Comp) []Comp {
	var out []Comp
	for _, c := range comps {
		if c.BldgAVPerSF < subject.BldgAVPerSF {
			c.Differences = Differences(subject, c)
			out = append(out, c)
			if len(out) == maxEvidence {
				break
			}
		}
	}
	return out
}

// landTolerance is how far the subject's land rate may sit from the comps'
// median before we ask the owner to check it. Land rates are usually one
// flat rate per neighborhood, so any real difference stands out.
const landTolerance = 0.01

// landFlag asks the owner to check the land assessment when its rate per land
// sq ft differs from similar homes'. It states the figures only.
// The comparison uses unrounded rates: at land rates under $1, rounding to
// cents alone could cross 1%.
func landFlag(s Home, comps []Comp, st Stats) []Flag {
	var rates []float64
	for _, c := range comps {
		if r := c.LandAVPerSF(); r > 0 {
			rates = append(rates, r)
		}
	}
	subj := s.LandAVPerSF()
	if subj <= 0 || len(rates) == 0 || abs64(subj/median(rates)-1) <= landTolerance {
		return nil
	}
	// At land rates under about $1, two rates more than 1% apart can round to
	// the same cents; show a third decimal then, so the figures differ as stated.
	subjS, medS := perSF(st.SubjectLandAVPerSF), perSF(st.MedianLandAVPerSF)
	if subjS == medS {
		subjS, medS = fmt.Sprintf("$%.3f", subj), fmt.Sprintf("$%.3f", median(rates))
	}
	return []Flag{{Field: "land", Message: fmt.Sprintf(
		"Please verify your land assessment. The Assessor records land AV of %s on %s sq ft of land, or %s per sq ft; "+
			"the median for the similar homes is %s per sq ft. The Assessor values land separately from the building, "+
			"usually at one rate per neighborhood.",
		money(s.LandAV), humanize.Comma(int64(s.LandSF)), subjS, medS)}}
}

func pct(x float64) string { return fmt.Sprintf("%.0f%%", 100*x) }

func money(x float64) string { return "$" + humanize.Comma(int64(x+0.5)) }

func perSF(x float64) string { return fmt.Sprintf("$%.2f", x) }

// decide applies the verdict rules and explains them in plain English.
func decide(r *Report) (string, []string) {
	st, s := r.Stats, r.Subject
	n, ev := st.NumComps, len(r.Evidence)
	var why []string

	if st.TooFewToCompare() {
		return Weak, []string{fmt.Sprintf(
			"We found only %d comparable home(s) in your neighborhood, too few to compare fairly.", n)}
	}

	gap := st.PctAboveMedian
	var rel string
	switch {
	case InLine(gap):
		rel = "in line with the median"
	case gap > 0:
		rel = pct(gap) + " above the median"
	default:
		rel = pct(-gap) + " below the median"
	}
	why = append(why, fmt.Sprintf(
		"Your home's building is assessed at %s per sq ft. The median of %d similar homes in your neighborhood is %s per sq ft, so yours is %s.",
		perSF(s.BldgAVPerSF), n, perSF(st.MedianBldgAVPerSF), rel))
	why = append(why, LandNote)
	why = append(why, fmt.Sprintf("%d of the %d comparable homes have a building assessed lower per sq ft than yours.",
		countBelow(*s, r.Comps), n))
	if r.Search.Level > 0 {
		why = append(why, "There weren't enough close matches, so we widened the search ("+
			r.Search.Description+"). That makes the comparison less precise.")
	}

	// Facts only: no recommendation about whether to appeal. The verdict is an
	// internal tier (styling, and whether we offer a packet), never shown as a label.
	if AboveMedian(gap) {
		why = append(why, fmt.Sprintf(
			"At similar homes' median building rate per sq ft, plus your land's assessed value of %s, your assessed value would be %s, which is %s lower than now.",
			money(s.LandAV), money(st.UniformityAV), money(st.PotentialAVReduction)))
	}
	g := Group(*st)
	// The caveat sits in the same sentence pair as the rate, so no surface can
	// quote one without the other (internal/golden's compliance tests check).
	why = append(why, fmt.Sprintf("In our test of past Board of Review decisions, homeowners who appealed on their own "+
		"with a building assessed %s got a reduction about %d%% of the time. "+
		"This is what happened to past appeals like this one; it is not a prediction for any one home.", groupWords[g], groupRates[g]))
	switch {
	case g == 0 && n >= minComps && ev >= minEvidence:
		return Strong, why
	case g <= 1 && ev >= minEvidence:
		if g == 0 {
			why = append(why, fmt.Sprintf("We found only %d similar homes, so this comparison rests on a small group.", n))
		}
		return Possible, why
	default:
		return Weak, why
	}
}

var groupWords = [5]string{
	"at least 1% above similar homes' median, and higher than at least three quarters of them,",
	"at least 1% above similar homes' median, but higher than fewer than three quarters of them,",
	"within 1% of similar homes' median",
	"more than 1% below similar homes' median, but higher than at least a quarter of them,",
	"more than 1% below similar homes' median, and higher than fewer than a quarter of them,",
}

// groupRates: pro se reduction rates by Group, tax year 2025, which the
// tuning never saw (docs/dev/tune-2026-11-26/backtest-2025.txt).
var groupRates = [5]int{92, 84, 67, 52, 25}

// InLine reports whether a gap is within ±1% of the median. Every surface
// describes such a home as in line with (or about equal to) similar homes.
func InLine(gap float64) bool { return abs64(gap) < possibleGap }

// AboveMedian reports whether a gap is far enough above the median (1% or
// more) that reports state the value at the median rate and the difference.
func AboveMedian(gap float64) bool { return gap >= possibleGap }

// CountBelow counts the comps whose building is assessed lower per sq ft
// than s's (strictly: equal rates don't count).
func CountBelow(s Home, comps []Comp) int { return countBelow(s, comps) }

func countBelow(s Home, comps []Comp) int {
	n := 0
	for _, c := range comps {
		if c.BldgAVPerSF < s.BldgAVPerSF {
			n++
		}
	}
	return n
}

func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Thresholds for the building-sqft outlier check.
const (
	minPeers     = 10
	sfHighFactor = 1.5
	sfLowFactor  = 1 / 1.5
)

// characteristicFlags looks for obvious record errors worth a second look.
// They're phrased as requests to verify, since the record may well be right.
func characteristicFlags(s Home, pool []Home) []Flag {
	var flags []Flag
	add := func(field, msg string, args ...any) {
		flags = append(flags, Flag{Field: field, Message: fmt.Sprintf(msg, args...)})
	}

	// Building sqft vs. same-group homes with the same rooms and bedrooms.
	var peerSF []float64
	for _, h := range pool {
		if h.PIN != s.PIN && h.NumCards == 1 && h.BldgSF > 0 &&
			classGroup[h.Class] == classGroup[s.Class] && h.Rooms == s.Rooms && h.Beds == s.Beds {
			peerSF = append(peerSF, h.BldgSF)
		}
	}
	if s.Rooms > 0 && len(peerSF) >= minPeers {
		med := median(peerSF)
		if s.BldgSF > sfHighFactor*med || s.BldgSF < sfLowFactor*med {
			add("bldg_sf",
				"Please verify your building square footage. The Assessor records %s sq ft; the median for %d similar homes in your neighborhood with %d rooms and %d bedrooms is %s sq ft. If the record is wrong, correcting it may matter more than comparables.",
				humanize.Comma(int64(s.BldgSF)), len(peerSF), s.Rooms, s.Beds, humanize.Comma(int64(med)))
		}
	}

	if s.Rooms > 0 && s.Beds > s.Rooms {
		add("beds", "Please verify the room counts: the record shows %d bedrooms but only %d rooms in total.", s.Beds, s.Rooms)
	}
	if s.FullBaths == 0 {
		add("fbath", "Please verify the bathroom count: the record shows no full bathrooms.")
	}
	if s.YearBuilt < 1800 || s.YearBuilt > time.Now().Year() {
		add("yrblt", "Please verify the year built: the record shows %d.", s.YearBuilt)
	}
	return flags
}
