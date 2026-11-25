# Tuning plan, written before the first trial (2026-11-26)

Committed before any trial ran, so the rules below can't be bent to fit the results.

## Data and split

- Board of Review appeals for in-scope homes (classes 202–210, 234, 278, 295) that ended in a decrease or no change, one row per PIN and tax year, as in `assessgap backtest`.
- **Tuning:** tax years 2023 and 2024 (`data/tune/tune.gob`, 368,881 PIN-years). **Holdout:** tax year 2025 (`data/tune/holdout.gob`, 219,816), written to a separate file that only `tune holdout` opens. `tune trial` and `tune signals` refuse a cache that holds 2025.
- **Population:** every trial is scored on the same appeals: those the current rule supports with 3 or more comps. If a trial finds fewer than 3 comps for one of them, it gets a neutral score (gap 0, or the 50th percentile), so a rule can't raise its AUC by dropping hard cases. Coverage is logged.
- **Check:** `tune verify` reproduces the engine backtest's comp count and gap for all 185,872 supported 2024 appeals in the 25c TSV, with 0 mismatches.

## Metrics (logged for every trial in `trials.tsv`)

- **Primary:** AUC of the score for reduction vs no change, pro se, 2023 and 2024 pooled. Also each year, and the change from the current rule.
- **Robustness:** the AUC change in each of 5 township groups (sorted township codes, round robin), pooled years.
- **Secondary:** attorney and all-filer AUC; the pro se win rate in the top 25% by score (with the share actually taken); Spearman correlation of score and % cut among pro se winners; tier calibration (the error of quoting 2023's tier win rates for 2024, weighted by 2024's tier shares); tier shares and win rates.

## Search (budget about 60 configurations, every one logged)

1. One change at a time from the current rule: distance weight, cap and hard limit; the number of comps; size and age tolerances; similarity weights (size, age, land, class, residence type, exterior wall, basement); new mismatch terms (central air, basement finish, garage); required matches (residence type, basement); the statistic (mean, 20% trimmed mean, similarity-weighted median, percentile rank).
2. Combine the changes that helped.
3. Context signals (logistic models, below).

## Choosing finalists (at most 3)

- **Robust:** the AUC gain is positive in 2023, in 2024, and in at least 4 of 5 township groups.
- **Finalist 1:** the robust rule with the highest pooled pro se AUC.
- **Finalist 2:** the best robust rule with a single change from the current rule, if it differs from finalist 1. This tells us whether a combination overfits where a single change doesn't.
- **Finalist 3:** a logistic model of the current gap plus the context signals, if its cross-validated AUC beats the gap-only model by more than 0.005. Otherwise, the best remaining robust rule.

## Holdout and ship gate

- `tune holdout` scores the current rule and each finalist on 2025 once. It logs each finalist in `holdout-log.tsv` and refuses one that is already there.
- 95% confidence intervals come from a bootstrap of the 2025 appeals by neighborhood (2,000 replicates, the same draws for every method, so differences are paired).
- **The engine changes only if** the best rule finalist's 2025 pro se AUC gain has a 95% interval entirely above 0, and its attorney AUC change is not significantly negative.
- A logistic model is never shipped as a score. At most, a strong signal becomes a *proposed* factual sentence for the maintainer to decide on.
