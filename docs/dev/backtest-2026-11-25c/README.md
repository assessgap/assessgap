# Backtest, 2026-11-25c: building-only comparison and distance

Same backtest as [docs/methodology.md](../../methodology.md): every in-scope Board of Review appeal for tax years 2024 and 2025 that ended in a decrease or no change (406,838 PIN-years; the 404,578 the engine supports are scored, and the rest are counted as unsupported), scored with the certified value the Board started from. Win = any reduction. "Pro se" = filed by the homeowner.

- `baseline/`: the method before this run (total AV ÷ building sq ft, no distance), on the full database.
- `new/`: everything in this run (building AV ÷ building sq ft, land added back in the uniformity figure, distance weight 1 per mile capped at 1 mile), on a copy of the same database rebuilt with the building/land columns. The raw tables are identical: the copy was made from the same file, read-only.
- Both runs use the same backtest code. Measurement was added first (the report-tier table, AUC, `-tsv`) and the baseline was re-run with it before any method change.

## Decision gate: passed

The building-only metric separates winners from losers **better**, not worse, so the metric was switched.

## Win rate by gap tier (the report's cut points)

| Gap from similar homes' median | Baseline: pro se n | win | New: pro se n | win | Baseline attorney win | New attorney win |
|---|--:|--:|--:|--:|--:|--:|
| ≥ +5% | 24,758 | 81.9% | 25,509 | **88.9%** | 66.8% | 75.6% |
| +1% to +5% | 23,156 | 77.6% | 21,186 | **83.9%** | 63.6% | 71.9% |
| in line (±1%) | 21,380 | 61.6% | 18,455 | **63.6%** | 47.4% | 49.8% |
| more than 1% below | 56,482 | 39.5% | 60,626 | **35.5%** | 31.0% | 27.3% |
| < 3 comps | 64 | 42.2% | 64 | 42.2% | 26.4% | 26.4% |

The spread from top tier to bottom grows from 42 to 53 points for self-filers, and from 36 to 48 for attorney filings.

## Separation (AUC of the gap, 3+ comps)

AUC is the chance that a randomly chosen appeal that won had a higher gap than one that lost (0.5 = chance).

| Method | Pro se | Attorney | All |
|---|--:|--:|--:|
| Baseline: total AV / sq ft, no distance | 0.731 | 0.687 | 0.699 |
| Building AV / sq ft, distance weight 0 | 0.769 | 0.728 | 0.740 |
| … distance weight 0.5 / mile | 0.784 | 0.742 | 0.754 |
| … distance weight 1 / mile (**chosen**) | **0.792** | **0.749** | **0.761** |
| … distance weight 2 / mile | 0.799 | 0.755 | 0.767 |

Pro se win rate by tier at each weight (≥5% / 1–5% / in line / below): w0 86.5 / 80.7 / 61.6 / 37.3; w0.5 88.2 / 82.6 / 63.1 / 36.1; w1 88.9 / 83.9 / 63.6 / 35.5; w2 89.4 / 84.9 / 64.1 / 34.9.

**Choice of weight.** Distance is no worse than without it at every weight tried, so it stays. The gain shrinks with each doubling (+0.015, +0.008, +0.007). I chose 1 per mile: it keeps most of the gain, and a mile still counts for no more than one noticeable difference in size or age. Property Tax Appeal Board decisions cite size and age more often than location, so similarity should stay in charge. A higher weight is a reasonable follow-up if the maintainer prefers; it moves comps further toward "nearest" and away from "most alike". The w0, w0.5 and w2 runs were made during tuning; the w1 numbers in the table are from the final run (`new/`). An earlier w1 run, before unknown locations counted as 1 mile, differed only in the third decimal (all 0.762).

## Homes that change tier

Of 404,578 appeals scored by both methods, **142,283 (35.2%) change tier**:

| Baseline tier \ new tier | ≥5% | 1–5% | in line | below |
|---|--:|--:|--:|--:|
| ≥5% | 50,673 | 10,951 | 4,190 | 10,360 |
| 1–5% | 15,213 | 27,030 | 11,090 | 16,278 |
| in line | 3,928 | 14,919 | 28,723 | 23,241 |
| below | 4,545 | 10,415 | 17,153 | 155,457 |

The building-only metric alone moves 30.0%. Distance alone, on top of it, moves 18.5% relative to the weight-0 run; the two partly cancel.

**Why: usually lot size.** From a 1-in-20 sample of each group (subject's own record):

| Group | Appeals | Land as share of total AV | Land sq ft (mean) |
|---|--:|--:|--:|
| Same tier | 262,295 | 22.1% | 8,418 |
| Other changes | 127,378 | 23.2% | 10,063 |
| ≥5% → below | 10,360 | **35.9%** | **17,973** |
| below → ≥5% | 4,545 | 22.6% | 8,263 |

Homes that drop from the top tier to below the median carry about 60% more of their value in land, on twice the lot. Their total looked high only because of the land, which is assessed at the neighborhood's rate. Examples (gap: total-based → building-based):

| PIN / tax year | Old gap | New gap | Land share of AV | Land sq ft |
|---|--:|--:|--:|--:|
| 01011140130000 / 2024 | +10.4% | −5.3% | 30% | 10,292 |
| 01022070100000 / 2024 | +11.9% | −18.6% | 50% | 20,325 |
| 01121000190000 / 2025 | +25.6% | −14.1% | 46% | 124,451 |
| 01192010140000 / 2024 | +12.1% | −10.0% | 43% | 413,820 |
| 32334170170000 / 2024 | +306.4% | −6.9% | 84% | 74,439 |

Homes that rise into the top tier have ordinary land shares. They rise because their comps are now nearer, or because the old comps' totals were inflated by big lots:

| PIN / tax year | Old gap | New gap | Land share of AV | Land sq ft |
|---|--:|--:|--:|--:|
| 01011190020000 / 2024 | −4.3% | +6.3% | 17% | 8,704 |
| 01163020020000 / 2025 | −11.5% | +13.4% | 8% | 43,560 |
| 29042020210000 / 2024 | −18.6% | +137.6% | 26% | 4,687 |

The task's example, PIN 13124210120000 (2026, mailed): land $14,200 on 4,000 sq ft ($3.55, the neighborhood rate) and building $52,800 on 1,389 sq ft = **$38.01** building-only, against $48.24 total ÷ building sq ft. Its six evidence comps are now $29.40–$35.72, all within 0.3 mile. The land check stays quiet, because its land rate matches its comps'.

The `sample.txt` outputs here end before their "example PINs" lists. Those named real homes under the engine's internal tiers, which are never shown as labels; the review removed them from the published copy.

## Share of homes that get an evidence report

| | Baseline | New |
|---|--:|--:|
| `sample` (2,000 random homes in townships still to open), strong + possible | 39.7% (408 + 386) | 38.9% (430 + 348) |
| Backtest appeals, strong + possible | 36.0% (145,735 of 404,579) | 34.0% (137,627 of 404,578) |

The land check fires for 2.1% of sampled homes (42 of 1,997). (A first version compared rates rounded to cents and fired for 79; at land rates under $1 a cent is more than 1%, so it now compares unrounded rates.)

## Thresholds

Kept at +5% and +1%. With the new metric the pro se win rate is 78.0% for 0–5% and about 90% from +5% to +20%, then 82% at +30% and above. +5% still marks where the rate levels off. The +1% cut separates 84% (1–5%) from 64% (within ±1%). The data doesn't make a clear case to move either.

The report sentences now quote 89% (≥5%), 84% (1–5%), 64% (within 1%) and 36% (more than 1% below), from the "By report tier cut points" table in `new/backtest.txt`.

## Independent check

A second agent wrote its own SQL from a plain description of the figures, without reading the engine code; I ran it unchanged. Results are in [docs/dev/2026-11-25c.md](../2026-11-25c.md#independent-verification).

## Reproduce

```sh
go run ./cmd/assessgap backtest -db path/to/cook.db -tsv appeals.tsv
go run ./cmd/assessgap sample -db path/to/cook.db
```

`sample` picks townships whose Board of Review dates aren't posted yet, so its population depends on the date. Both runs here were on 2026-11-25.
