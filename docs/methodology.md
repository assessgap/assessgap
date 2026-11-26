# Methodology

AssessGap compares a home's **building assessed value per square foot of building area** with similar homes nearby in the same Cook County Assessor neighborhood. That is an assessment-uniformity comparison. It does not estimate what any property would sell for, and it is not an appraisal.

All data is public, from the [Cook County open data portal](https://datacatalog.cookcountyil.gov/): parcel universe (including each parcel's location), addresses, assessed values (total, building and land), residential characteristics, sales, and Assessor and Board of Review appeal outcomes.

## Why the building, not the total

We compare the building part of your assessment, per square foot, because the Assessor values land separately at a neighborhood rate.

Every assessed value has two parts: the **building** (improvement) and the **land**. The Assessor values land at a rate per square foot of land, usually one flat rate per neighborhood. Dividing the *total* by building square feet would make a home on a bigger lot look over-assessed even when its building is assessed at the same rate as its neighbors'. That is also what professionals file: Property Tax Appeal Board decisions compare the improvement assessment per square foot of living area.

Land is checked on its own: if the subject's land AV per square foot of land differs from the similar homes' median by more than 1%, the report asks the owner to verify it.

## Which homes are supported
Classes 202–210, 234, 278 and 295 (single-family homes and townhomes). Condos (299) are not supported: a condo unit's assessed value is the building's value times the unit's percentage of ownership, so in about 95% of buildings every unit is assessed at the same rate and a unit-to-unit comparison finds nothing. Two-to-six-flats, multi-building PINs, PINs without building square footage and PINs whose record doesn't split the value into building and land (or shows a $0 building, or a split that doesn't add up to the total within $1; none did in 2026) are also unsupported, and the report says why.

## Finding similar homes
| Step | Rule |
|---|---|
| Subject value | The certified value for the year if the Assessor has certified the township, otherwise the mailed value. The building and land split comes from the same stage. The report says which. |
| Pool | Every supported single-building home with square footage, an assessed value and a building/land split (building > $0) in the same Assessor neighborhood and year. |
| Search | Same class, building size within ±25%, built within ±15 years. If that finds fewer than 8 homes, widen step by step: same class group (one-story, multi-story, townhome; never across groups), then ±35% / ±25 years, then ±50% / ±40 years. The report says how far it widened. Same neighborhood and same class group are never relaxed. |
| Distance | Straight-line (haversine) miles between the parcel centers in the parcel universe. |
| Similarity | Score = 4·min(1, miles) + 1·\|Δsqft\|/sqft + \|Δyears\|/15 + 0.5·\|Δbaths\| + 0.5·min(1, \|Δland\|/land) + 0.5 for each mismatch in residence type or exterior wall + 0.25 if the class differs. An unknown location counts as 1 mile. The 30 most similar are kept; among equally similar homes, the nearer comes first. Within the size and age bands above, nearness counts most. |
| Statistics | Median building AV per sq ft of the similar homes; the subject's gap from that median; how many of the similar homes have a building assessed lower per sq ft (strictly lower); the value at the median rate (median × the subject's building square feet + the subject's own land AV). |
| Lower-assessed homes | The most similar homes (up to 6) whose building AV per sq ft is below the subject's. These are highlighted, and each gets a factual "differences from your home" line. |
| Data checks | Building square footage far from similar neighbors, more bedrooms than rooms, zero full baths, missing or implausible year built, and a land rate more than 1% from the similar homes' median: shown as "worth double-checking". |

The weights, the number of homes kept and the tiers below were chosen on 2026-11-26 with an honest time split: rules were tuned on Board of Review appeals from tax years 2023 and 2024 only, and the chosen rule was then checked once on tax year 2025, which the search never saw. See [docs/dev/2026-11-26-tune.md](dev/2026-11-26-tune.md) and [Backtest](#backtest) below.

## The evidence grid
For the subject and each similar home the report shows: address and PIN, distance, class, stories, construction (exterior wall), building sq ft, year built, bedrooms and baths, basement and finish, central air, fireplaces, garage, land sq ft, building AV, land AV, total AV and building AV per sq ft. Characteristics are the Assessor's published words, reworded plainly ("Formal Rec Room" → "finished"); a bare numeric code is never shown.

"Differences from your home" lists, for each lower-assessed home, only differences in the features Property Tax Appeal Board decisions weigh when they compare homes (as reasons for less weight, or as adjustments): distance (whenever both locations are known), class, stories, construction, living area (5% or more), age (10 years or more), full and half baths, basement type and finish, central air, fireplaces and garage. It states facts only.

## How we describe results
We state facts and don't tell anyone whether to appeal. Internally, a report is in one of three tiers, which only controls styling and whether the evidence report is offered:

- at least three quarters of the similar homes have a building assessed lower per sq ft, with 8 or more similar homes and at least 3 assessed lower;
- at least half of them assessed lower, and at least 3;
- everything else.

The tiers follow how many similar homes are assessed lower, not the size of the gap from the median: in the backtest, a home's rank among its similar homes separated reductions from no change better than its gap did. Every report still states the gap and the value at the median rate.

## Backtest
We run the method on in-scope Board of Review appeals that ended in a decrease or no change, using the certified value the Board started from (the value at the stage appealed), and compare with the actual outcome. Win = any reduction.

**How the method was chosen (2026-11-26).** About 60 candidate rules (for example, distance and size weights, the number of homes kept, size and age bands, required matches, and how the subject is compared with the group) were scored on tax years 2023 and 2024 only (368,881 PIN-years). A rule had to do better in both years and across groups of townships. The chosen rule was then scored once on tax year 2025 (219,816 PIN-years), which the search never saw. All 2025 figures below are from that check.

| Tax year 2025, self-filed (68,092 appeals with 3+ similar homes) | AUC |
|---|--:|
| Previous method: gap from the median, 20 homes, distance 1 per mile | 0.789 |
| This method: rank among 30 homes, nearness weighted most | **0.817** |

The gain is +0.028 (95% interval +0.025 to +0.030, from resampling neighborhoods). For attorney filings it is 0.751 → 0.777. On the tuning years the gain was +0.031, so nearly all of it held on unseen data. AUC is the chance that a randomly chosen appeal that got a reduction ranked higher than one that didn't (0.5 = chance).

By the report's tiers, tax year 2025:

| Similar homes with a building assessed lower per sq ft | Self-filed: share of appeals | Got a reduction | Attorney: share | Got a reduction |
|---|--:|--:|--:|--:|
| at least three quarters | 19.8% | 91.7% | 17.6% | 81.4% |
| half to three quarters | 26.5% | 81.0% | 25.2% | 68.2% |
| a quarter to half | 26.0% | 54.1% | 26.9% | 41.9% |
| fewer than a quarter | 27.7% | 24.6% | 30.3% | 18.8% |

On 2023–2024 the same tiers gave 92.7%, 79.5%, 49.7% and 20.5% for self-filers; 2025 ran a few points higher in the lower tiers.

By the gap from the median, 2025, self-filed (the same comparison):

| Building AV/sq ft vs similar homes' median | Self-filed n | Got a reduction | Avg cut |
|---|--:|--:|--:|
| below −10% | 9,897 | 22.4% | 8.8% |
| −10% to −5% | 10,018 | 32.0% | 7.4% |
| −5% to 0% | 16,989 | 54.1% | 6.9% |
| 0% to 5% | 16,252 | 81.0% | 6.9% |
| 5% to 10% | 7,259 | 91.0% | 7.9% |
| 10% to 20% | 4,747 | 90.4% | 9.8% |
| 20% to 30% | 1,320 | 88.3% | 12.6% |
| 30% or more | 1,610 | 84.3% | 18.8% |

What it shows: homes assessed higher than most of their nearest similar homes got reductions far more often, in every year and for both kinds of filer. A bigger gap mostly means a bigger cut.

Caveats:
- These are outcomes for people who chose to appeal, who usually had a reason to. They are not a prediction for any one home.
- Characteristics and locations are the latest on record, not those of the appeal year. A characteristic corrected after an appeal therefore shows up in the earlier years too, which likely raises every method's AUC a little, most in the oldest year.
- A Board decrease can come from reasons other than uniformity (vacancy, sales, errors); we don't split by reason.
- Reports mostly use mailed 2026 values; the backtest used certified values.
- The previous method's distance weight was chosen on 2024–2025 data, so in the 2025 check it had a small head start.

Reproduce with `go run ./cmd/assessgap backtest -years 2025` against a full database (`-tsv` writes one line per appeal). The comparison with the previous method, with its confidence intervals, is `go run ./tools/tune prep` and then `go run ./tools/tune holdout -out <empty dir>` (see the dev log).

## Known gaps
- No dollar estimate of tax savings yet: the open data has no current tax rates (they stop in 2013). PTAXSIM is the likely source.
- Board of Review only; Assessor-stage appeals aren't covered yet.
