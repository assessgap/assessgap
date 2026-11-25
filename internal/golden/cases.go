package golden

// Case is one golden example: a name for the file and the arguments after
// "assessgap check -json" (a PIN, and sometimes -year).
//
// The PINs come from the frozen fixture (testdata/fixture): Assessor
// neighborhoods 27080 (Oak Park, certified 2026 values), 39082 (Worth,
// certified) and 71280 (Jefferson, mailed only). Each covers a distinct
// report shape; testdata/README.md explains how they were chosen.
type Case struct {
	Name string
	Args []string
}

// Cases are the check goldens, in testdata/golden/check/<name>.txt.
// Names were chosen in 2026-11-25b, when the gap was total AV per sq ft. Since
// 2026-11-25c it is building AV per sq ft, so some names describe the old
// shape; the comments give the current gap.
var Cases = []Case{
	// Where the home sits against the median.
	{"well-above-mailed", []string{"13331030130000"}},      // +50% (was +32%), 20 comps, recorded sales
	{"well-above-certified", []string{"24293040050000"}},   // +10%, split-level 234, certified
	{"above-5-to-10", []string{"13331030140000"}},          // +14% (was +7.5%)
	{"slightly-above", []string{"13331030180000"}},         // +7.7% (was +2%): now the top tier
	{"above-but-few-comps", []string{"16171280200000"}},    // +23% with 7 comps: lower tier
	{"in-line", []string{"13331110210000"}},                // +0.8%: within ±1%
	{"below-none-lower", []string{"13331020180000"}},       // −29%, no lower-assessed homes, top-20 cut
	{"below-widened-to-group", []string{"13331050110000"}}, // −1.5%, search level 1
	{"land-rate-differs", []string{"13332060380000"}},      // land $1.79 vs $1.85 per land sq ft: the land check
	{"few-comps-4", []string{"13332250110000"}},            // 4 comps after full widening
	{"too-few-comps", []string{"13332280060000"}},          // 1 comp
	{"townhome-identical-3", []string{"16171270490000"}},   // 295 with 3 near-identical comps (building −0.03%, was a tie on total AV)
	{"townhome-below", []string{"24283020360000"}},         // 295, −33%
	{"widened-level-2", []string{"16171180290000"}},        // ±35% / ±25 years
	{"widened-level-3", []string{"16171220340000"}},        // ±50% / ±40 years
	{"tie-with-subject", []string{"13331050170000"}},       // 2 comps at exactly the subject's AV/sq ft
	{"class-202-small", []string{"13331050040000"}},        // one-story under 1,000 sq ft
	{"exactly-8-comps", []string{"13332270190000"}},        // the minimum before widening; also a big lot: +18% on total AV, −11% on the building
	{"check-sqft-high", []string{"16171100190000"}},        // building sq ft far above peers
	{"check-no-full-baths", []string{"16171210300000"}},    // zero full baths
	{"check-beds-over-rooms", []string{"24294090450000"}},  // more bedrooms than rooms
	{"sales-with-deed-types", []string{"24294030030000"}},  // 3 recorded sales
	{"no-address", []string{"24294070120000"}},             // supported home with no address
	{"year-2025-certified", []string{"13331050130000", "-year", "2025"}},
	{"latest-year-2023", []string{"13331120380000"}}, // no 2026 value; no parcel row
	// Unsupported and not found.
	{"unsupported-condo", []string{"13331120411001"}},
	{"unsupported-multi-card", []string{"13331060090000"}},
	{"unsupported-two-flat", []string{"13331010100000"}},
	{"unsupported-exempt", []string{"13331000020000"}},
	{"unsupported-class-241", []string{"13331050070000"}},
	{"unknown-pin", []string{"99999999999999"}},
	{"known-pin-missing-year", []string{"13331030130000", "-year", "1999"}},
}

// PacketPINs get a golden of the evidence report's text, in
// testdata/golden/pdf/<name>.txt.
var PacketPINs = []Case{
	{"well-above-mailed", []string{"13331030130000"}},
	{"check-no-full-baths", []string{"16171210300000"}},
	{"few-comps-4", []string{"13332250110000"}},
	{"in-line", []string{"13331110210000"}},           // within 1%: no difference stated, like the web page
	{"land-rate-differs", []string{"13332060380000"}}, // the land "please verify" item
}
