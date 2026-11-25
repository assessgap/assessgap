# AssessGap

**Free tools to help Cook County homeowners appeal their own property assessments.**

AssessGap compares your home's assessed value per square foot with similar homes in your Assessor neighborhood, using public county data, and shows how Board of Review appeals have gone for homeowners near you. If similar homes are assessed lower than yours, it produces an evidence report you can attach to your own appeal.

- Website: **https://assessgap.com** (free, no sign-up)
- How it works: [docs/methodology.md](docs/methodology.md), including a backtest against ~400,000 Board of Review decisions
- Board of Review filing facts, with sources: [docs/bor-filing-facts.md](docs/bor-filing-facts.md)

Filing an appeal with the Cook County Board of Review is free, and you don't need a lawyer or a paid service. Paid services charge $49 or more, or 30–50% of your savings, for this kind of analysis. We think it should be free.

> AssessGap is not affiliated with the Cook County Assessor, the Board of Review, or any government office. It is not a law firm and does not give legal advice, and it is not an appraisal. You file, sign and present your own appeal.

## Status
Early. The website and the analysis work today. Planned next: a single `assessgap` CLI that works straight from the county's open-data API (no big download), and Assessor-stage appeals.

## Run it yourself
Requires Go 1.26+. Everything is pure Go (SQLite via `modernc.org/sqlite`), so there's no cgo.

```sh
# 1. Download a small sample (one area of Jefferson Township) and build a database
go run ./cmd/ingest -data data-sample -pin-prefix 1328 all
go run ./cmd/build -db data-sample/cook.db

# 2. A report for one home, as JSON
go run ./cmd/check -db data-sample/cook.db 13281000270000

# 3. The evidence report PDF (flags go before the PIN)
go run ./cmd/packet -db data-sample/cook.db -o report.pdf 13281000270000

# 4. The website, at http://127.0.0.1:8080
go run ./cmd/server -cook data-sample/cook.db -app data-sample/app.db
```

For the whole county, drop `-pin-prefix` and use `-data data`: about 45 minutes and 9 GB. `ops/refresh.sh` rebuilds the full database safely (fresh download, validation, atomic swap).

## Layout
| Path | What |
|---|---|
| `cmd/ingest`, `cmd/build` | Download county datasets from Socrata; build typed, indexed SQLite tables |
| `internal/comps` | The comparison engine: similar homes, statistics, data checks |
| `internal/bor` | Board of Review township calendar and official links |
| `internal/packet` | The evidence report PDF (a small PDF writer, no dependencies) |
| `internal/web`, `cmd/server` | The website: Go `net/http`, templ, htmx, Tailwind |
| `cmd/check`, `cmd/compseval` | Report as JSON; coverage sample and backtest |
| `ops/` | Refresh and deploy scripts (Caddy + systemd on a small VPS) |

Templates are [templ](https://templ.guide); generated `*_templ.go` and `static/app.css` are committed. After editing templates or styles: `scripts/tailwind.sh` once, then `go generate ./internal/web/...`.

## Contributing
Issues and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md). Ideas that would help most: the CLI, Assessor-stage appeals, tax-rate data for a dollar estimate, a Spanish translation, and other Illinois counties.

## License
[MIT](LICENSE) © 2026 AssessGap LLC
