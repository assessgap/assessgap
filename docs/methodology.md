# Methodology

AssessGap compares a home's **assessed value per square foot of building area** with similar homes in the same Cook County Assessor neighborhood. That is an assessment-uniformity comparison. It does not estimate what any property would sell for, and it is not an appraisal.

All data is public, from the [Cook County open data portal](https://datacatalog.cookcountyil.gov/): parcel universe, addresses, assessed values, residential characteristics, sales, and Assessor and Board of Review appeal outcomes.

## Which homes are supported
Classes 202–210, 234, 278 and 295 (single-family homes and townhomes). Condos (299) are not supported: a condo unit's assessed value is the building's value times the unit's percentage of ownership, so in about 95% of buildings every unit is assessed at the same rate and a unit-to-unit comparison finds nothing. Two-to-six-flats, multi-building PINs and PINs without building square footage are also unsupported, and the report says why.

## Finding similar homes
| Step | Rule |
|---|---|
| Subject value | The certified value for the year if the Assessor has certified the township, otherwise the mailed value. The report says which. |
| Pool | Every supported single-building home with square footage and assessed value in the same Assessor neighborhood and year. |
| Search | Same class, building size within ±25%, built within ±15 years. If that finds fewer than 8 homes, widen step by step: same class group (one-story, multi-story, townhome; never across groups), then ±35% / ±25 years, then ±50% / ±40 years. The report says how far it widened. |
| Similarity | Distance = 4·\|Δsqft\|/sqft + \|Δyears\|/15 + 0.5·\|Δbaths\| + 0.5·min(1, \|Δland\|/land) + 0.5 for each mismatch in residence type, exterior wall or basement + 0.25 if the class differs. The 20 nearest are kept. |
| Statistics | Median AV/sqft of the similar homes; the subject's gap from that median; the value at the median rate (median × the subject's square footage). |
| Lower-assessed homes | The nearest similar homes (up to 6) whose AV/sqft is below the subject's. These are the ones highlighted in the report. |
| Data checks | Building square footage far from similar neighbors, more bedrooms than rooms, zero full baths, missing or implausible year built: shown as "worth double-checking". |

## How we describe results
We state facts and don't tell anyone whether to appeal. Internally, a report is in one of three tiers, which only controls styling and whether the evidence report is offered:

- at least 5% above the median, with 8 or more similar homes and at least 3 assessed lower;
- at least 1% above the median with at least 3 assessed lower;
- everything else.

## Backtest
We ran the method on every supported Board of Review appeal for tax years 2024 and 2025 that ended in a decrease or no change (406,838 PIN-years), using the certified value the Board started from, and compared with the actual outcome. Win = any reduction.

| Gap from similar homes' median | Self-filed n | Got a reduction | Avg cut | Attorney n | Got a reduction |
|---|--:|--:|--:|--:|--:|
| below −10% | 12,435 | 27.2% | 9.2% | 32,606 | 22.8% |
| −10% to −5% | 17,714 | 33.0% | 7.8% | 39,851 | 26.2% |
| −5% to 0% | 34,322 | 52.7% | 7.2% | 75,610 | 41.2% |
| 0% to 5% | 36,547 | 71.5% | 7.2% | 78,908 | 56.5% |
| 5% to 10% | 13,306 | 82.1% | 8.4% | 26,460 | 67.3% |
| 10% to 20% | 7,453 | 82.7% | 10.1% | 16,057 | 67.5% |
| 20% to 30% | 1,877 | 82.2% | 13.3% | 4,358 | 66.7% |
| 30% or more | 2,122 | 77.7% | 19.9% | 4,541 | 61.3% |

What it shows: homes above their similar homes' median got reductions far more often than homes below it, in both years and for both kinds of filer. Above about +5% the chance levels off near 82% for self-filers, and a bigger gap mostly means a bigger cut. Self-filers did better than attorney filings in every band, though attorneys file more borderline cases.

Caveats:
- These are outcomes for people who chose to appeal, who usually had a reason to. They are not a prediction for any one home.
- Characteristics are the latest on record, not those of the appeal year.
- A Board decrease can come from reasons other than uniformity (vacancy, sales, errors); we don't split by reason.
- Reports mostly use mailed 2026 values; the backtest used certified values.

Reproduce with `go run ./cmd/compseval backtest` against a full database.

## Known gaps
- No dollar estimate of tax savings yet: the open data has no current tax rates (they stop in 2013). PTAXSIM is the likely source.
- Board of Review only; Assessor-stage appeals aren't covered yet.
