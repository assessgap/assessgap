# AssessGap

**Free tools to help Cook County homeowners appeal their own property assessments.**

AssessGap compares the building part of your home's assessed value, per square foot, with similar homes nearby in your Assessor neighborhood (land is assessed separately, at a neighborhood rate), using public county data, and shows how Board of Review appeals have gone for homeowners near you. If similar homes are assessed lower than yours, it produces an evidence report you can attach to your own appeal.

- Website: **https://assessgap.com** (free, no sign-up)
- How it works: [docs/methodology.md](docs/methodology.md), including how the method was tuned on about 370,000 Board of Review decisions from 2023–24 and checked on about 220,000 from 2025
- Board of Review filing facts, with sources: [docs/bor-filing-facts.md](docs/bor-filing-facts.md)

Filing an appeal with the Cook County Board of Review is free, and you don't need a lawyer or a paid service. Paid services charge $49 or more, or 30–50% of your savings, for this kind of analysis. We think it should be free.

> AssessGap is not affiliated with the Cook County Assessor, the Board of Review, or any government office. It is not a law firm and does not give legal advice, and it is not an appraisal. You file, sign and present your own appeal.

## Status
Early. The website, the `assessgap` command (which can check a home straight from the county's open data API, no big download) and the free evidence report PDF work today. Planned next: configurable comparisons ([data inventory](docs/data-inventory.md)) and Assessor-stage appeals.

## Install the command

- **Download:** prebuilt `assessgap` binaries for macOS, Linux and Windows (Intel and ARM) are on the [Releases](https://github.com/assessgap/assessgap/releases) page once a release is published. Unpack the archive and put `assessgap` on your `PATH`. `checksums.txt` lists each archive's SHA-256.
- **With Go 1.26+:** `go install github.com/assessgap/assessgap/cmd/assessgap@latest`

`assessgap version` prints the version, commit and build date.

### Quick check from your terminal

No database or download is needed for one home. `check` fetches just that home's neighborhood from the county's open data API (live mode):

```sh
assessgap check 13281000270000                 # your 14-digit PIN, from your tax bill
assessgap report 13281000270000 -o report.pdf  # the free evidence report PDF
assessgap township jefferson                   # your township's Board of Review window
```

Live mode is for checking a home at a time; searching by address needs a local database (below).

## Run it yourself
Requires Go 1.26+. Everything is pure Go (SQLite via `modernc.org/sqlite`), so there's no cgo.

Everything is one command, `assessgap` (`go install ./cmd/assessgap`, or `go run ./cmd/assessgap` as below). Flags may go before or after the address or PIN.

```sh
# 1. Download a small sample (one area of Jefferson Township) and build a database
go run ./cmd/assessgap db sync -data data-sample -pin-prefix 1328
export ASSESSGAP_DB=data-sample/cook.db   # or pass -db to each command

# 2. Compare one home with similar homes (add -json for the full report as JSON)
go run ./cmd/assessgap check 13281000270000
go run ./cmd/assessgap check 3134 N Luna Ave

# 3. The free evidence report PDF
go run ./cmd/assessgap report 13281000270000 -o report.pdf

# 4. Board of Review filing windows, for every township or one
go run ./cmd/assessgap township
go run ./cmd/assessgap township jefferson

# 5. The website, at http://127.0.0.1:8080
go run ./cmd/assessgap serve -app data-sample/app.db
```

**No download needed for one home:** if there's no database (no `-db`, no `$ASSESSGAP_DB`, no `./data/cook.db`), `assessgap check <PIN>` and `assessgap report <PIN> -o x.pdf` fetch just that home's neighborhood from the county's open data API (about 10 requests and a few MB, usually a few seconds). Force it with `-live`. Searching by address still needs a local database.

Without `-db` or `$ASSESSGAP_DB`, `check`, `report`, `serve`, `backtest`, `sample` and `live-eval` use `./data/cook.db`; the `db` steps take their paths from `-db` or `-data` only. For the whole county, run `assessgap db sync` with no flags: about 45 minutes and 9 GB. `ops/refresh.sh` rebuilds the full database safely (fresh download, validation, atomic swap) using `assessgap db fetch|load|build|validate|finalize`. `assessgap backtest -years 2025` reproduces the methodology's 2025 tables against a full database (`tools/tune` reproduces the tuning; see the methodology).

## Layout
| Path | What |
|---|---|
| `cmd/assessgap`, `internal/cli` | The `assessgap` command and its subcommands |
| `internal/ingest`, `internal/socrata` | Download county datasets from Socrata; build typed, indexed SQLite tables (`build.sql`) |
| `internal/live` | Live mode: one home's inputs from the API into an in-memory database |
| `internal/comps` | The comparison engine: similar homes, statistics, data checks |
| `internal/search` | Find a parcel by address or PIN (the website and the CLI) |
| `internal/bor` | Board of Review township calendar and official links |
| `internal/packet` | The evidence report PDF (a small PDF writer, no dependencies) |
| `internal/web` | The website: Go `net/http`, templ, htmx, Tailwind |
| `internal/fixture`, `internal/golden`, `testdata/` | A frozen slice of county data for tests, and golden outputs that make any change to a homeowner-facing number a reviewable diff ([testdata/README.md](testdata/README.md)) |
| `tools/` | Rebuild the fixture (`fixture`); reproduce the tuning and 2025 holdout (`tune`); the share image (`ogimage`); PR checks (`prcheck`, `prloc`) |
| `ops/` | Refresh and deploy scripts (Caddy + systemd on a small VPS); `logs.sh` (live logs) and `stats.sh` (daily traffic counts from the access log, no IPs or PINs) |

Templates are [templ](https://templ.guide); generated `*_templ.go` and `static/app.css` are committed. After editing templates or styles: `scripts/tailwind.sh` once, then `go generate ./internal/web/...`.

## Contributing
Issues and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md). Ideas that would help most: Assessor-stage appeals, tax-rate data for a dollar estimate, a Spanish translation, and other Illinois counties.

## License
[MIT](LICENSE) © 2026 AssessGap LLC
