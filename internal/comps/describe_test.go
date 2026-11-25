package comps

import "testing"

func TestDifferences(t *testing.T) {
	subject := Home{Class: "203", BldgSF: 1400, YearBuilt: 1950, FullBaths: 1, HalfBaths: 1,
		TypeResd: "1 Story", ExtWall: "Masonry", Basement: "Full", BsmtFin: "Unfinished",
		Air: "Central A/C", Fireplaces: 0, Garage: "0 cars"}
	tests := []struct {
		name string
		c    Comp
		want string
	}{
		{"same features, nearby", Comp{Home: subject, Miles: 0.04}, "under 0.1 mi away"},
		{"no location, nothing different", Comp{Home: subject, Miles: -1}, "No differences in the recorded features we compare"},
		{
			// 1,463 sq ft is 4.5% larger and 1959 is 9 years later: both under the thresholds.
			"just under the size and age thresholds",
			Comp{Home: with(subject, func(h *Home) { h.BldgSF, h.YearBuilt = 1463, 1959 }), Miles: 0.26},
			"0.3 mi away",
		},
		{
			// 1,232 sq ft is 12% smaller; 1962 is 12 years later.
			"every difference",
			Comp{Miles: 1.26, Home: Home{Class: "204", BldgSF: 1232, YearBuilt: 1962, FullBaths: 2, HalfBaths: 0,
				TypeResd: "2 Story", ExtWall: "Frame", Basement: "Full", BsmtFin: "Formal Rec Room",
				Air: "No Central A/C", Fireplaces: 1, Garage: "2 cars"}},
			"1.3 mi away · class 204 vs 203 · 2-story vs 1-story · frame vs masonry · 12% smaller · " +
				"built 12 years later · 1 more full bath · 1 fewer half bath · finished basement vs unfinished · " +
				"no central air vs central air · 1 fireplace vs none · 2-car garage vs none",
		},
		{
			// A different basement type says so once; its finish isn't compared.
			"basement type, and no garage vs the subject's",
			Comp{Miles: -1, Home: with(subject, func(h *Home) { h.Basement, h.BsmtFin = "Slab", "Formal Rec Room" })},
			"slab vs full basement",
		},
		{
			"unknown values are left out, never shown as codes",
			Comp{Miles: -1, Home: with(subject, func(h *Home) { h.TypeResd, h.Garage, h.Air = "3", "", "" })},
			"No differences in the recorded features we compare",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Differences(subject, tt.c); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
	garaged := with(subject, func(h *Home) { h.Garage, h.Fireplaces = "1.5 cars", 2 })
	if got := Differences(garaged, Comp{Home: subject, Miles: -1}); got != "no fireplace vs 2 · no garage vs 1.5-car" {
		t.Errorf("subject with garage and fireplaces: got %q", got)
	}
}

func with(h Home, f func(*Home)) Home {
	f(&h)
	return h
}
