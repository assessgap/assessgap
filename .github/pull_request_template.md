## Summary (plain language)

<!-- Two or three sentences a homeowner or a new contributor could follow. What does this change, and for whom? -->

## What changed

<!-- The user-facing before and after (site, CLI, PDF, data), then the mechanism: which code, which data, how it works. -->

## Why

<!-- The problem or goal, and the alternatives you considered and why you didn't pick them. -->

## Numbers impact

<!--
Required; a check fails the PR while this is empty.
Either write "None, goldens unchanged" (testdata/golden/ has no diff), or list every golden that changed
and why each change is correct, e.g.:
  check/well-above-mailed: stats.median_comp_av_per_sf 18.95 → 19.10, because <reason>; checked by <how>.
The diff of testdata/golden/ is what reviewers read. If the fixture was rebuilt, say why.
-->

## Visual proof

<!-- Before and after screenshots for any UI change (desktop and phone width), or "N/A: <reason>". -->

## Verification

<!--
Commands you ran and what you saw (scripts/check.sh at least), and manual steps a reviewer can repeat,
e.g. "go run ./cmd/assessgap check -db data-fixture/cook.db 13331030130000" (see testdata/README.md).
-->

## Tests

<!--
Tests added, changed or deleted, and why (docs/testing.md).
For each new test, confirm it fails without the change: say how you checked (reverted the fix, broke the code).
-->

## Risk and rollback

<!--
Required; a check fails the PR while this is empty.
Risk level (low / medium / high), what could break and for whom (homeowners, the county API, the server),
and how to back it out (revert the commit? a data refresh? a deploy step?).
-->

## AI disclosure

<!-- Was any of this written by an AI tool? Which parts, and what did a human review? Write "None" if not. -->
