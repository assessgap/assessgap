# AssessGap

**Free tools to help Cook County homeowners appeal their own property assessments.**

AssessGap compares your home's assessed value per square foot with similar homes in your Assessor neighborhood, using public county data, and shows how Board of Review appeals have gone for homeowners near you. If similar homes are assessed lower than yours, it produces an evidence report you can attach to your own appeal.

- Website: **https://assessgap.com** (free, no sign-up)
- How it works: [docs/methodology.md](docs/methodology.md), including a backtest against ~400,000 Board of Review decisions
- Board of Review filing facts, with sources: [docs/bor-filing-facts.md](docs/bor-filing-facts.md)

Filing an appeal with the Cook County Board of Review is free, and you don't need a lawyer or a paid service. Paid services charge $49 or more, or 30–50% of your savings, for this kind of analysis. We think it should be free.

> AssessGap is not affiliated with the Cook County Assessor, the Board of Review, or any government office. It is not a law firm and does not give legal advice, and it is not an appraisal. You file, sign and present your own appeal.

## Status
Early. The website, the `assessgap` command and the free evidence report PDF work today. Planned next: checking a home straight from the county's open-data API (no big download), and Assessor-stage appeals.

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

Without `-db` or `$ASSESSGAP_DB`, commands use `./data/cook.db`. For the whole county, run `assessgap db sync` with no flags: about 45 minutes and 9 GB. `ops/refresh.sh` rebuilds the full database safely (fresh download, validation, atomic swap) using `assessgap db fetch|load|build|validate|finalize`. `assessgap backtest` and `assessgap sample` reproduce the numbers in the methodology against a full database.

## Layout
| Path | What |
|---|---|
| `cmd/assessgap`, `internal/cli` | The `assessgap` command and its subcommands |
| `internal/ingest`, `internal/socrata` | Download county datasets from Socrata; build typed, indexed SQLite tables (`build.sql`) |
| `internal/comps` | The comparison engine: similar homes, statistics, data checks |
| `internal/search` | Find a parcel by address or PIN (the website and the CLI) |
| `internal/bor` | Board of Review township calendar and official links |
| `internal/packet` | The evidence report PDF (a small PDF writer, no dependencies) |
| `internal/web` | The website: Go `net/http`, templ, htmx, Tailwind |
| `ops/` | Refresh and deploy scripts (Caddy + systemd on a small VPS) |

Templates are [templ](https://templ.guide); generated `*_templ.go` and `static/app.css` are committed. After editing templates or styles: `scripts/tailwind.sh` once, then `go generate ./internal/web/...`.

## Contributing
Issues and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md). Ideas that would help most: Assessor-stage appeals, tax-rate data for a dollar estimate, a Spanish translation, and other Illinois counties.

## License
[MIT](LICENSE) © 2026 AssessGap LLC
