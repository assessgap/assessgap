package comps

import "math"

// TaxInputs is one scenario for turning an assessed-value change into a
// yearly tax-bill change. It is NOT yet wired into Report: Cook County's open
// data has no current tax rates or equalizer (see docs/methodology.md),
// and we don't invent them.
type TaxInputs struct {
	Equalizer  float64 // state equalization factor for the tax year (e.g. from IDOR)
	Rate       float64 // composite tax rate for the parcel's tax code, as a fraction (Clerk's 7.25% → 0.0725)
	Exemptions float64 // total exemption EAV on the bill (homeowner, senior, ...); 0 if unknown
}

// billChange is the bill difference between assessed values av and av-cut:
// rate × (taxable EAV before − taxable EAV after), where taxable EAV is
// max(0, AV × equalizer − exemptions). Exemptions only matter when they push
// taxable EAV to zero; otherwise the change is cut × equalizer × rate.
func (t TaxInputs) billChange(av, cut float64) float64 {
	taxable := func(a float64) float64 { return math.Max(0, a*t.Equalizer-t.Exemptions) }
	return t.Rate * (taxable(av) - taxable(av-cut))
}

// EstTaxChange returns the low end of the estimated yearly bill change for an
// AV reduction of cut on a parcel assessed at av, over the given scenarios
// (e.g. the last few years' rates and equalizers while the current year's
// aren't published). It rounds to the nearest $10. It returns 0 if there are no
// scenarios or no reduction.
func EstTaxChange(av, cut float64, scenarios []TaxInputs) float64 {
	if cut <= 0 || len(scenarios) == 0 {
		return 0
	}
	cut = math.Min(cut, av)
	low := math.Inf(1)
	for _, s := range scenarios {
		low = math.Min(low, s.billChange(av, cut))
	}
	return 10 * math.Round(low/10)
}
