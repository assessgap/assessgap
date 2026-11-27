package packet

import (
	"fmt"
	"math"
	"time"

	"github.com/assessgap/assessgap/internal/bor"
	"github.com/assessgap/assessgap/internal/comps"
	"github.com/assessgap/assessgap/internal/format"
)

// Page 1: the property, the comparison and the uniformity figure.

func (p *pager) summary(generated time.Time) {
	r, s, st := p.r, p.r.Subject, p.r.Stats
	p.newPage(fmt.Sprintf("Assessment comparison: tax year %d", r.Year))
	p.para(margin, contentW, regular, 9, fmt.Sprintf(
		"Prepared %s from public Cook County Assessor data. Figures are assessed values (AV) as published by the Assessor.",
		format.LongDate(bor.InChicago(generated))))

	p.heading("Property")
	p.keyValues(propertyRows(r))

	p.heading("Comparison with similar homes")
	p.body(fmt.Sprintf(
		"The property's building is assessed at %s per square foot of building area (building AV %s ÷ %s sq ft). "+
			"The median of %d similar homes in the same Assessor neighborhood is %s per square foot, so the property is %s the median.",
		format.PerSF(s.BldgAVPerSF), format.Money(s.BldgAV), format.Num(s.BldgSF), st.NumComps, format.PerSF(st.MedianBldgAVPerSF), gapPhrase(st.PctAboveMedian)))
	p.body("The building part of the assessment is compared, per square foot, because the Assessor values land " +
		"separately at a neighborhood rate per square foot of land.")
	below := comps.CountBelow(*s, r.Comps)
	p.body(fmt.Sprintf("%d of the %d comparable homes have a building assessed lower per square foot than the property. "+
		"The property ranks at the %s percentile of the comparable homes by building AV per square foot.",
		below, st.NumComps, ordinal(int(math.Round(st.SubjectPercentile)))))
	if r.Search != nil {
		p.body(fmt.Sprintf("How comparable homes were chosen: %s (Assessor neighborhood %s), excluding multi-building "+
			"parcels and homes without recorded square footage or a building assessment. The %d most similar by distance "+
			"(weighted most), size, age, baths, lot, design and construction (the nearer home first when two are equally "+
			"similar) are listed on page 2.", r.Search.Description, r.Nbhd, st.NumComps))
	}

	p.heading("Uniformity figure")
	p.body(uniformityText(*s, *st))

	if len(r.Flags) > 0 {
		p.heading("Please verify")
		for _, f := range r.Flags {
			p.bullet(f.Message)
		}
	}

	p.y += 6
	p.boxed(r.Disclaimer + " " + NotAffiliated)
}

// propertyRows are page 1's property table.
func propertyRows(r *comps.Report) [][2]string {
	s := r.Subject
	stage := "Certified: the Assessor's value after Assessor appeals"
	if r.AVStage != "certified" {
		stage = "Mailed: the Assessor's initial value; not yet certified, and it may change"
	}
	addr := format.TitleCase(s.Address)
	if r.City != "" {
		addr += ", " + format.TitleCase(r.City)
	}
	return [][2]string{
		{"Address", addr},
		{"PIN", format.PIN(r.PIN)},
		{"Property class", r.Class},
		{"Township", r.Township},
		{"Tax year", fmt.Sprint(r.Year)},
		{"Assessed value", fmt.Sprintf("%s (building %s + land %s)", format.Money(s.AV), format.Money(s.BldgAV), format.Money(s.LandAV))},
		{"Value stage", stage},
		{"Building", fmt.Sprintf("%s sq ft, built %s, %s, %s", format.Num(s.BldgSF), yearOr(s.YearBuilt),
			plural(fmt.Sprint(s.Beds), "bedroom"), plural(format.Baths(s.Baths()), "bath"))},
	}
}

// uniformityText applies the comps' median building rate to the property
// and compares the result with its current assessed value.
func uniformityText(s comps.Home, st comps.Stats) string {
	applied := fmt.Sprintf(
		"Applying the comparable homes' median building rate of %s per square foot to the property's %s square feet, "+
			"and adding the property's land AV of %s, gives an assessed value of %s",
		format.PerSF(st.MedianBldgAVPerSF), format.Num(s.BldgSF), format.Money(s.LandAV), format.Money(st.UniformityAV))
	switch {
	case st.StatesReduction():
		return fmt.Sprintf("%s. That is %s less than the current assessed value of %s.",
			applied, format.Money(st.PotentialAVReduction), format.Money(s.AV))
	case st.PotentialAVReduction >= 1:
		// Within 1% of the median: every surface calls this about equal, so no difference is stated.
		return fmt.Sprintf("%s, about the same as the current assessed value of %s: the building's rate is within 1%% of the median.",
			applied, format.Money(s.AV))
	default:
		return fmt.Sprintf("%s, which is not below the current assessed value of %s.", applied, format.Money(s.AV))
	}
}
