package comps

import (
	"fmt"
	"sort"
	"time"

	"github.com/dustin/go-humanize"
)

const (
	// possibleGap: within ±1% of the median is "in line" on every surface, and
	// from +1% the reports state the value at the median rate.
	possibleGap = 0.01
	minEvidence = 3
	maxEvidence = 6
)

// Tier bands on the share of similar homes whose building is assessed lower
// per sq ft than the subject's. Ranking a home among its comps this way
// separated Board of Review outcomes better than its gap from the median
// (docs/dev/2026-11-26-tune.md). Strictly lower, so a home assessed the same
// as its neighbors doesn't rise a band.
const (
	strongShare   = 0.75
	possibleShare = 0.50
	lowShare      = 0.25
)

// ShareLower is the share of comps assessed lower per sq ft than the subject.
func (st Stats) ShareLower() float64 {
	if st.NumComps == 0 {
		return 0
	}
	return float64(st.CompsLower) / float64(st.NumComps)
}

// ShareBand is the band index (0 = at least three quarters lower … 3 = fewer
// than a quarter) a share falls in.
func ShareBand(share float64) int {
	switch {
	case share >= strongShare:
		return 0
	case share >= possibleShare:
		return 1
	case share >= lowShare:
		return 2
	default:
		return 3
	}
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

	if n < minEvidence {
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
	band := ShareBand(st.ShareLower())
	why = append(why, fmt.Sprintf("In our test of past Board of Review decisions, homeowners who appealed on their own "+
		"when %s of similar homes had a building assessed lower per sq ft got a reduction about %d%% of the time.",
		shareBandWords[band], shareBandRates[band]))
	switch {
	case band == 0 && n >= minComps && ev >= minEvidence:
		return Strong, why
	case band <= 1 && ev >= minEvidence:
		if band == 0 {
			why = append(why, fmt.Sprintf("We found only %d similar homes, so this comparison rests on a small group.", n))
		}
		return Possible, why
	default:
		return Weak, why
	}
}

var shareBandWords = [4]string{"at least three quarters", "half to three quarters", "a quarter to half", "fewer than a quarter"}

// shareBandRates: pro se reduction rates by band, tax year 2025, which the
// tuning never saw (docs/dev/tune-2026-11-26/backtest-2025.txt).
var shareBandRates = [4]int{92, 81, 54, 25}

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
