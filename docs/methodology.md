# Methodology

AssessGap compares a home's **building assessed value per square foot of building area** with similar homes nearby in the same Cook County Assessor neighborhood. That is an assessment-uniformity comparison. It does not estimate what any property would sell for, and it is not an appraisal.

All data is public, from the [Cook County open data portal](https://datacatalog.cookcountyil.gov/): parcel universe (including each parcel's location), addresses, assessed values (total, building and land), residential characteristics, sales, and Assessor and Board of Review appeal outcomes.

## Why the building, not the total

We compare the building part of your assessment, per square foot, because the Assessor values land separately at a neighborhood rate.

Every assessed value has two parts: the **building** (improvement) and the **land**. The Assessor values land at a rate per square foot of land, usually one flat rate per neighborhood. Dividing the *total* by building square feet would make a home on a bigger lot look over-assessed even when its building is assessed at the same rate as its neighbors'. That is also what professionals file: Property Tax Appeal Board decisions compare the improvement assessment per square foot of living area.

Land is checked on its own: if the subject's land AV per square foot of land differs from the similar homes' median by more than 1%, the report asks the owner to verify it.

## Which homes are supported
Classes 202–210, 234, 278 and 295 (single-family homes and townhomes). Condos (299) are not supported: a condo unit's assessed value is the building's value times the unit's percentage of ownership, so in about 95% of buildings every unit is assessed at the same rate and a unit-to-unit comparison finds nothing. Two-to-six-flats, multi-building PINs, PINs without building square footage and PINs whose record doesn't split the value into building and land (or shows a $0 building) are also unsupported, and the report says why.

## Finding similar homes
| Step | Rule |
|---|---|
| Subject value | The certified value for the year if the Assessor has certified the township, otherwise the mailed value. The building and land split comes from the same stage. The report says which. |
| Pool | Every supported single-building home with square footage, an assessed value and a building/land split (building > $0) in the same Assessor neighborhood and year. |
| Search | Same class, building size within ±25%, built within ±15 years. If that finds fewer than 8 homes, widen step by step: same class group (one-story, multi-story, townhome; never across groups), then ±35% / ±25 years, then ±50% / ±40 years. The report says how far it widened. Same neighborhood and same class group are never relaxed. |
| Distance | Straight-line (haversine) miles between the parcel centers in the parcel universe. |
| Similarity | Score = 4·\|Δsqft\|/sqft + \|Δyears\|/15 + 0.5·\|Δbaths\| + 0.5·min(1, \|Δland\|/land) + 0.5 for each mismatch in residence type, exterior wall or basement + 0.25 if the class differs + 1·min(1, miles). An unknown location counts as 1 mile. The 20 most similar are kept; among equally similar homes, the nearer comes first. |
| Statistics | Median building AV per sq ft of the similar homes; the subject's gap from that median; the value at the median rate (median × the subject's building square feet + the subject's own land AV). |
| Lower-assessed homes | The most similar homes (up to 6) whose building AV per sq ft is below the subject's. These are highlighted, and each gets a factual "differences from your home" line. |
| Data checks | Building square footage far from similar neighbors, more bedrooms than rooms, zero full baths, missing or implausible year built, and a land rate more than 1% from the similar homes' median: shown as "worth double-checking". |

The distance weight (1 per mile, capped at 1 mile) was chosen with the backtest: the pro se AUC was 0.769 without distance, 0.784 at 0.5, 0.792 at 1 and 0.799 at 2. See [docs/dev/backtest-2026-11-25c/README.md](dev/backtest-2026-11-25c/README.md).

## The evidence grid
For the subject and each similar home the report shows: address and PIN, distance, class, stories, construction (exterior wall), building sq ft, year built, bedrooms and baths, basement and finish, central air, fireplaces, garage, land sq ft, building AV, land AV, total AV and building AV per sq ft. Characteristics are the Assessor's published words, reworded plainly ("Formal Rec Room" → "finished"); a bare numeric code is never shown.

"Differences from your home" lists, for each lower-assessed home, only differences in the features Property Tax Appeal Board decisions weigh when they compare homes (as reasons for less weight, or as adjustments): distance (always), class, stories, construction, living area (5% or more), age (10 years or more), full and half baths, basement type and finish, central air, fireplaces and garage. It states facts only.

## How we describe results
We state facts and don't tell anyone whether to appeal. Internally, a report is in one of three tiers, which only controls styling and whether the evidence report is offered:

- at least 5% above the median, with 8 or more similar homes and at least 3 assessed lower;
- at least 1% above the median with at least 3 assessed lower;
- everything else.

## Backtest
We ran the method on every supported Board of Review appeal for tax years 2024 and 2025 that ended in a decrease or no change (406,838 PIN-years), using the certified value the Board started from, and compared with the actual outcome. Win = any reduction.

| Building AV/sq ft vs similar homes' median | Self-filed n | Got a reduction | Avg cut | Attorney n | Got a reduction |
|---|--:|--:|--:|--:|--:|
| below −10% | 17,625 | 22.4% | 8.8% | 45,848 | 18.8% |
| −10% to −5% | 18,473 | 31.7% | 7.5% | 42,050 | 23.3% |
| −5% to 0% | 32,802 | 51.1% | 7.0% | 75,720 | 39.8% |
| 0% to 5% | 31,367 | 78.0% | 7.1% | 65,922 | 64.4% |
| 5% to 10% | 12,701 | 90.1% | 8.2% | 23,315 | 77.5% |
| 10% to 20% | 7,828 | 89.6% | 10.2% | 15,161 | 76.6% |
| 20% to 30% | 2,271 | 88.1% | 12.8% | 4,621 | 74.4% |
| 30% or more | 2,709 | 82.0% | 19.2% | 5,753 | 65.9% |

By the report's own cut points, self-filers got a reduction about 89% of the time at +5% or more, 84% at +1% to +5%, 64% within ±1%, and 36% more than 1% below.

How well the gap separates reductions from no change (AUC; 0.5 = no better than chance): 0.792 for self-filers and 0.749 for attorney filings, against 0.731 and 0.687 when we compared total AV per square foot without distance.

What it shows: homes above their similar homes' median got reductions far more often than homes below it, in both years and for both kinds of filer. Above about +5% the chance levels off near 89% for self-filers, and a bigger gap mostly means a bigger cut. Self-filers did better than attorney filings in every band, though attorneys file more borderline cases.

Caveats:
- These are outcomes for people who chose to appeal, who usually had a reason to. They are not a prediction for any one home.
- Characteristics and locations are the latest on record, not those of the appeal year.
- A Board decrease can come from reasons other than uniformity (vacancy, sales, errors); we don't split by reason.
- Reports mostly use mailed 2026 values; the backtest used certified values.

Reproduce with `go run ./cmd/assessgap backtest` against a full database (`-tsv` writes one line per appeal).

## Known gaps
- No dollar estimate of tax savings yet: the open data has no current tax rates (they stop in 2013). PTAXSIM is the likely source.
- Board of Review only; Assessor-stage appeals aren't covered yet.
