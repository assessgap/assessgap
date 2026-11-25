# Contributing

Thanks for helping. A few ground rules keep this useful and safe for homeowners:

- **No market-value language.** We compare assessed values only. Don't add text that states what a property is worth or calls anything an appraisal (Illinois licenses appraisers).
- **Facts, not legal advice.** Describe data and published procedures. Don't tell users what to argue or whether to appeal.
- **Cite official sources** for anything about Board of Review or Assessor procedures, and note the date you checked.
- **Numbers must be right.** Every figure a homeowner sees comes from county data through tested code, and any change to one is reviewed as a diff.
- Keep it simple: standard library first, small packages, table-driven tests. [AGENTS.md](AGENTS.md) has the conventions (it's written for AI coding agents, but applies to everyone).

Bigger changes: open an issue first so we can agree on the approach.

## The flow

1. **Branch** from `main`.
2. **Change, and test where it matters.** Read [docs/testing.md](docs/testing.md) first: tests are for costly regressions (the comparison math, dates, parsing, security behaviour, the build), every test must fail without its change, and report-level behaviour belongs in the goldens. Iterate with `go test ./internal/<package>`.
3. **Run `scripts/check.sh`** before pushing. It runs gofmt, `go vet`, golangci-lint, the tests (including the goldens, on a committed fixture of real county data, so nothing skips), a check that generated templ and CSS output is up to date, and the build. It prints only failures. CI runs the same script.
4. **If a golden fails**, your change moved something a homeowner sees. If that's intended, run `go test ./internal/golden ./internal/web -run Golden -update` and read `git diff testdata/golden`. If it isn't, you found a bug.
5. **Open a pull request using the template.** "Numbers impact" (either "None, goldens unchanged" or each changed golden and why it's right) and "Risk and rollback" are required; a check fails the PR until they're filled in. A bot adds a table of test vs production lines changed.
6. **Review.** Reviewers read the `testdata/golden/` diff as the record of what changed for homeowners, and every number in it needs an explanation in the description.

The test fixture (`testdata/fixture/`) is frozen public county data; see [testdata/README.md](testdata/README.md) for what's in it and how to rebuild it. A rebuild updates the goldens in the same pull request.
