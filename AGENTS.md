# AGENTS.md

AssessGap helps Cook County homeowners appeal their own property assessments by comparing assessed value per square foot with similar homes, using public county data. The worst bug it can have is **quietly showing a homeowner a wrong number**. Everything below serves that.

## Checks

- **Gate:** `scripts/check.sh` before every push. It runs gofmt, a vague-name check, `go vet`, golangci-lint (`.golangci.yml`), `go test`, a generated-files check and `go build`, and prints only failures. CI runs the same script.
- **While iterating**, run only what you touched: `go test ./internal/comps`, `go test ./internal/comps -run Synthetic -v`, `go vet ./internal/web`.
- **Goldens:** if a change moves a number or text in the report JSON, the evidence report's text or the report page's data, `go test ./internal/golden ./internal/web` fails with `case (PIN): field: old → new`. Not covered: how templates place that data, the filing-window text and the chart, so check those by running the site. If the change is intended, run `go test ./internal/golden ./internal/web -run Golden -update`, read `git diff testdata/golden`, and explain each change under "Numbers impact" in the PR. Never update goldens to make a failure go away without understanding it.
- Never skip the gate, and don't disable a lint rule or add `//nolint` without a one-line reason.

## Layout

| Path | What |
|---|---|
| `cmd/assessgap`, `internal/cli` | The `assessgap` command |
| `internal/comps` | The comparison engine: pool, search widening, similarity, statistics, data checks |
| `internal/ingest` (`build.sql`), `internal/socrata`, `internal/store` | Download county datasets and build the SQLite database |
| `internal/live` | Live mode: one home's inputs from the county API |
| `internal/search`, `internal/bor`, `internal/packet` | Address/PIN lookup, Board of Review calendar, the evidence report PDF |
| `internal/web` (`views/*.templ`) | The website |
| `internal/fixture`, `internal/golden`, `testdata/` | The frozen test fixture and the goldens ([testdata/README.md](testdata/README.md)) |
| `tools/fixture` | Rebuilds the fixture from a full database |
| `ops/` | Refresh and deploy scripts |

Generated and committed: `internal/web/views/*_templ.go` (`go tool templ generate -path internal/web/views`) and `internal/web/static/app.css` (`scripts/tailwind.sh`, then `go generate ./internal/web/...`).

## First-class concerns: every change must consider these

1. **Number accuracy.** Every figure must be traceable to county data through code that a test or golden covers. A change to the engine, `build.sql`, rounding or formatting must show up as a golden diff, and the PR must say why each changed number is right. Don't invent data (tax rates, values) the county doesn't publish.
2. **The scope line.** State facts about assessed values and published procedures only. No market value, no "appraisal" (except to say this isn't one), no verdict labels ("strong case"), no "you should appeal", no legal advice, and never filing for anyone. The tiers in `comps` only drive styling and whether the PDF is offered; they are never shown as labels. `internal/golden/compliance_test.go` scans every output for banned phrases; add new ones there.
3. **Privacy.** PINs and addresses are never sent to third parties (no analytics, no third-party scripts, fonts or cookies; the CSP is `'self'` only). Request logs stay on the server, and `ops/stats.sh` reports counts without IPs or PINs. Signup emails stay in the app database and are never logged.
4. **Load on the county's API.** Live mode is for single checks from the CLI. The website reads a local database and must never call the API per request. Batch jobs use a full download, not live mode.
5. **The small server.** About 1 GB of RAM and one vCPU; at most 4 concurrent heavy requests (reports, PDFs, search) via `busy` in `internal/web/limit.go`. Don't load whole tables into memory per request or add work to the request path that scales with the county.

## Code style

- **Reuse before reimplementing.** Extend the existing function (`search.ParsePIN`, `ingest.Columns`, `comps.ScopeClasses`) instead of writing a parallel one.
- **No vague names.** No packages or files named `util`, `utils`, `helper(s)`, `common` or `misc`; `scripts/check.sh` rejects them. Name things for what they hold.
- **Small packages** with one job, standard library first. No new dependency without a reason in the PR.
- **Comments say why**, in a line or two; the code says what.
- Return errors (wrapped with `%w`, compared with `errors.Is`); no `panic` outside `main`, tests and init-time impossibilities. Write output to the `io.Writer` you were given, never `fmt.Print*` from `internal/`.
- `gofmt`, idiomatic Go, table-driven tests where cases differ.

## Tests

Read [docs/testing.md](docs/testing.md). In short: test only where a silent regression is costly; every test must fail if its change is reverted and must hardcode its expected values; prefer the goldens for report-level behaviour; use the real-data fixture for goldens and the synthetic builder for engine edge cases; no tests of copy, constants or third-party code, except the compliance phrases.

## Pull requests

Use `.github/pull_request_template.md`. "Numbers impact" and "Risk and rollback" are required, and a workflow checks them. Say so if an AI wrote the change.

**Dependent PRs use `gh stack`** ([github/gh-stack](https://gh.io/stacks)). When one branch builds on another, never stack by hand by basing a branch on a PR branch and merging one into the other.
- Start or adopt a stack with `gh stack init <bottom> <next> …`. Add layers with `gh stack add`, then open or update every PR with `gh stack submit --auto`, which creates drafts.
- Fix a lower layer **on that layer**, then run `gh stack rebase` and `gh stack push` (or `gh stack sync`) so every layer above picks it up. Don't merge one stack branch into another, and don't push to a layer some other way.
- The maintainer merges the whole stack at once with `gh stack merge`, which is all-or-nothing. Agents never merge.
- Independent changes stay as separate branches off `main`, not a stack.
- **Generated files** (`internal/web/static/app.css`, `*_templ.go`) conflict whenever two branches change templates. Never resolve them by hand: take either side, rerun `scripts/check.sh` to regenerate them, and commit the result.

**Visual proof.** Any PR that changes what users see includes before and after screenshots, desktop and mobile ([CONTRIBUTING.md](CONTRIBUTING.md#visual-proof)):
- run `scripts/screenshots.sh` against `main` and against the branch (and `scripts/pdf-pages.sh` for the evidence report), then look at every image;
- push the images to the `pr-assets` branch under `pr-<number>/`;
- link them in the PR body as `https://raw.githubusercontent.com/assessgap/assessgap/pr-assets/pr-<number>/<file>.png`;
- never commit screenshots to code branches, and show no address or PIN that isn't already public in `testdata/golden` or the docs (`-redact`).
