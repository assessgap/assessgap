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
// Names were chosen in 2026-11-25b, when the gap was total AV per sq ft and
// 20 comps were kept. Since 2026-11-25c it is building AV per sq ft, and since
// 2026-11-26 30 comps, chosen nearest, with tiers by the gap and then the share
// assessed lower. Some names describe an old shape; the comments give the
// current one (gap, comps assessed lower of all comps, tier).
var Cases = []Case{
	// Where the home sits against the median.
	{"well-above-mailed", []string{"13331030130000"}},      // +45%, 30 of 30 lower, recorded sales: strong
	{"well-above-certified", []string{"24293040050000"}},   // +10%, 27 of 30, split-level 234, certified: strong
	{"above-5-to-10", []string{"13331030140000"}},          // +9.7%, 21 of 30: above the median but under 3/4 lower, possible
	{"slightly-above", []string{"13331030180000"}},         // +4.8%, 18 of 30: possible
	{"above-but-few-comps", []string{"16171280200000"}},    // +23%, 6 of 7: under 8 comps, possible
	{"in-line", []string{"13331110210000"}},                // +0.8%, 19 of 30: in line, so weak though most are lower
	{"below-none-lower", []string{"13331020180000"}},       // −30%, 0 of 30, the 30-comp cut
	{"below-widened-to-group", []string{"13331050110000"}}, // search level 1; now −0.3% (in line), 15 of 30 (widened-level-3 covers below after widening)
	{"land-rate-differs", []string{"13332060380000"}},      // land $1.79 vs $1.85 per land sq ft: the land check
	{"few-comps-4", []string{"13332250110000"}},            // 4 comps after full widening
	{"too-few-comps", []string{"13332280060000"}},          // 1 comp
	{"townhome-identical-3", []string{"16171270490000"}},   // 295 with 3 near-identical comps (building −0.03%, was a tie on total AV)
	{"townhome-below", []string{"24283020360000"}},         // 295, −33%
	{"widened-level-2", []string{"16171180290000"}},        // ±35% / ±25 years
	{"widened-level-3", []string{"16171220340000"}},        // ±50% / ±40 years; −9%, 5 of 30
	{"tie-with-subject", []string{"13331050170000"}},       // 2 comps at exactly the subject's AV/sq ft
	{"class-202-small", []string{"13331050040000"}},        // one-story under 1,000 sq ft
	{"exactly-8-comps", []string{"13332270190000"}},        // the minimum before widening; also a big lot: +18% on total AV, −11% on the building
	{"check-sqft-high", []string{"16171100190000"}},        // building sq ft far above peers
	{"check-no-full-baths", []string{"16171210300000"}},    // zero full baths
	{"check-beds-over-rooms", []string{"24294090450000"}},  // more bedrooms than rooms
	{"sales-with-deed-types", []string{"24294030030000"}},  // 3 recorded sales
	{"no-address", []string{"24294070120000"}},             // supported home with no address; +7.4%, 25 of 30: strong
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
