package comps

import (
	"fmt"
	"sort"
	"time"

	"github.com/dustin/go-humanize"
)

// Verdict thresholds on how far the subject's AV/sqft sits above the comp median.
const (
	strongGap   = 0.05 // backtest: pro se win rate plateaus ~82% from +5% (docs/methodology.md)
	possibleGap = 0.01 // 0–5% above median: ~72% pro se win rate; below 1% is "in line"
	minEvidence = 3
	maxEvidence = 6
)

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

func computeStats(subject Home, comps []Comp) Stats {
	vals := make([]float64, len(comps))
	below, equal := 0, 0
	for i, c := range comps {
		vals[i] = c.AVPerSF
		switch {
		case c.AVPerSF < subject.AVPerSF:
			below++
		case c.AVPerSF == subject.AVPerSF:
			equal++
		}
	}
	st := Stats{NumComps: len(comps), SubjectAVPerSF: round(subject.AVPerSF, 2)}
	if len(comps) == 0 {
		return st
	}
	med := median(vals)
	st.MedianAVPerSF = round(med, 2)
	st.PctAboveMedian = round(subject.AVPerSF/med-1, 4)
	st.SubjectPercentile = round(100*(float64(below)+0.5*float64(equal))/float64(len(comps)), 1)
	st.UniformityAV = round(med*subject.BldgSF, 0)
	st.PotentialAVReduction = max(0, round(subject.AV-st.UniformityAV, 0))
	return st
}

// evidenceComps picks the most similar comps assessed lower per sqft than the
// subject: the ones a homeowner would cite. comps must be sorted nearest first.
func evidenceComps(subject Home, comps []Comp) []Comp {
	var out []Comp
	for _, c := range comps {
		if c.AVPerSF < subject.AVPerSF {
			out = append(out, c)
			if len(out) == maxEvidence {
				break
			}
		}
	}
	return out
}

func pct(x float64) string { return fmt.Sprintf("%.0f%%", 100*x) }

func money(x float64) string { return "$" + humanize.Comma(int64(x+0.5)) }

func perSF(x float64) string { return fmt.Sprintf("$%.2f", x) }

// decide applies the verdict rules and explains them in plain English.
func decide(r *Report) (string, []string) {
	st, s := r.Stats, r.Subject
	n, ev := st.NumComps, len(r.Evidence)
	var why []string

	if n < minEvidence {
		return Weak, []string{fmt.Sprintf(
			"We found only %d comparable home(s) in your neighborhood, too few to compare fairly.", n)}
	}

	gap := st.PctAboveMedian
	var rel string
	switch {
	case abs64(gap) < 0.01:
		rel = "in line with the median"
	case gap > 0:
		rel = pct(gap) + " above the median"
	default:
		rel = pct(-gap) + " below the median"
	}
	why = append(why, fmt.Sprintf(
		"Your home is assessed at %s per sq ft. The median of %d similar homes in your neighborhood is %s per sq ft, so yours is %s.",
		perSF(s.AVPerSF), n, perSF(st.MedianAVPerSF), rel))
	why = append(why, fmt.Sprintf("%d of the %d comparable homes are assessed lower per sq ft than yours.",
		countBelow(*s, r.Comps), n))
	if r.Search.Level > 0 {
		why = append(why, "There weren't enough close matches, so we widened the search ("+
			r.Search.Description+"). That makes the comparison less precise.")
	}

	// Facts only: no recommendation about whether to appeal. The verdict is an
	// internal tier (styling, and whether we offer a packet), never shown as a label.
	if gap >= possibleGap {
		why = append(why, fmt.Sprintf(
			"At similar homes' median rate per sq ft, your assessed value would be %s, which is %s lower than now.",
			money(st.UniformityAV), money(st.PotentialAVReduction)))
	}
	switch {
	case gap >= strongGap:
		why = append(why, "In our test of past Board of Review decisions, homeowners who appealed on their own with homes assessed at least 5% above similar homes got a reduction about 82% of the time.")
	case gap >= possibleGap:
		why = append(why, "In our test of past Board of Review decisions, homeowners who appealed on their own with homes assessed 0–5% above similar homes got a reduction about 72% of the time.")
	default:
		why = append(why, "In our test of past Board of Review decisions, homeowners who appealed on their own with homes assessed at or below similar homes got a reduction 27–53% of the time.")
	}
	switch {
	case gap >= strongGap && n >= minComps && ev >= minEvidence:
		return Strong, why
	case gap >= possibleGap && ev >= minEvidence:
		if gap >= strongGap {
			why = append(why, fmt.Sprintf("We found only %d similar homes, so this comparison rests on a small group.", n))
		}
		return Possible, why
	default:
		return Weak, why
	}
}

func countBelow(s Home, comps []Comp) int {
	n := 0
	for _, c := range comps {
		if c.AVPerSF < s.AVPerSF {
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
