# Testing

The costliest bug in AssessGap is quietly showing a homeowner a wrong number. Tests exist to catch that, and a few other regressions that are expensive and silent. Everything else is proven by running the change.

The suite is deliberately small. Keep it that way: a test that doesn't guard something costly is code someone has to maintain, and a test that can't fail is worse than none, because it looks like coverage.

## Where tests are worth it

- **Comparison math and statistics** (`internal/comps`): the pool, the search widening, the similarity distance, the median, the gap, percentiles, the value at the median rate, the lower-assessed homes, the tiers, the data checks.
- **Backtest figures** (`internal/cli/eval.go`): the numbers quoted in `docs/methodology.md` and in report text.
- **Board of Review dates and windows** (`internal/bor`): open, upcoming, closed, days left, time zones.
- **PIN and address parsing and search** (`internal/search`): a wrong match shows someone else's home.
- **The evidence report PDF** (`internal/packet`): it renders, fits its pages, and shows the same figures as the report.
- **Live mode matching the database** (`internal/live`): the same inputs as a full build.
- **HTTP security behaviour** (`internal/web`): rate limits, the client IP behind the proxy, headers, input validation, status codes.
- **Ingest and build correctness** (`internal/ingest`, `internal/refresh`): what gets downloaded, what `build.sql` keeps, and the refresh's validation and swap.

## Prefer the goldens for report-level behaviour

If the question is "what does the report say for this home?", the answer belongs in a golden, not a hand-written assertion. `internal/golden` runs `assessgap check -json` and the PDF text on the frozen fixture, and `internal/web` does the same for the report page; see [testdata/README.md](../testdata/README.md). A new report shape gets a new case in `internal/golden/cases.go`. A change that moves a number shows up as a diff in `testdata/golden/`, which the reviewer reads and the PR's "Numbers impact" section explains.

Use the two data sources for different jobs, and don't mix them:

- **The real-data fixture** (`internal/fixture`, `testdata/fixture/`) for goldens and for tests that need a realistic database. It is frozen; only a deliberate rebuild changes it.
- **The synthetic builder** (`internal/comps/synthetic_test.go`) for engine edge cases real data can't reach cleanly: 0 or 2 comparable homes, an even-count median, ties, zero square feet, no characteristics row. Expected numbers are worked out by hand in a comment.

## Every test must

1. **Fail if its change is reverted.** Before you commit a new test, break the code it guards (or stash the fix) and watch it fail. Say so in the PR.
2. **Hardcode its expected values.** Never compute the expectation with production code (`want := median(xs)`, `want := round(st.MedianAVPerSF*sf, 0)`). Write the number, and show the arithmetic in a comment if it isn't obvious.
3. **Live at the one cheapest layer that owns the behaviour.** The median is tested in `comps`, not again through the CLI and the website. The CLI's tests cover argument parsing and address resolution; the website's cover HTTP behaviour.
4. **Be table-driven when cases differ only in inputs and expected outputs.** One row per distinct behaviour, named for it. Don't add rows that exercise the same branch, and don't branch inside the loop body.
5. **Be deterministic.** No network (the one real-API test skips under `-short` and in CI), no wall clock (pass a `time.Time`), no `time.Sleep`, no map iteration order in assertions.

## Don't write tests that

- Reimplement production logic, or use it as the oracle.
- Assert nothing meaningful: a non-nil result, a non-empty string, "no error" alone, `len(x) > 0` where the exact count is knowable.
- Assert copy or wording (button text, headings, help text). The one exception is below.
- Pin constants, defaults, thresholds or lists (`minComps == 8`, the number of townships). If a constant matters, test the behaviour it controls.
- Test the standard library or third-party packages (SQLite, `encoding/csv`, templ, `net/http`).
- Promise coverage they don't have: a case whose input never reaches the branch it's named after.
- Reach into state the code never exposes, or need production code added only for tests (exported test hooks, `ForTests` functions, flags that change behaviour under test). Package-internal tests calling unexported functions are fine; that's ordinary Go.
- Duplicate a golden: if a golden already pins a report's numbers, don't also assert them by hand.

Mocks and fakes: don't mock our own database or engine. Use the fixture or the synthetic builder, which run the real SQL. Fake only what crosses the process boundary (the county API, the clock), and keep a fake consistent with the real thing.

## The one exception: compliance wording

AssessGap's legal scope is a costly regression: it must never state a market value, call anything an appraisal, label a home with a verdict ("strong case", "you should appeal"), or show a price for AssessGap itself. So a small number of tests assert that these phrases **never** appear in what a homeowner sees: the report JSON's text fields, the report page, and the evidence report PDF.

They live in one place, `internal/golden/compliance_test.go`, and scan every golden case's output. Add a phrase there when the scope line moves; don't scatter banned-word checks across packages.

## Running tests

```sh
scripts/check.sh                                   # the full gate (what CI runs)
go test ./internal/comps                           # one package while iterating
go test ./internal/comps -run Synthetic -v         # one test
go test ./internal/golden ./internal/web -run Golden -update   # rewrite goldens, deliberately
```

Adapted from the test anti-patterns in [Orca](https://github.com/stablyai/orca) (`docs/agents-md-refs/test-antipatterns.md`, MIT licence, © Stably AI).
