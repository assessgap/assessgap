# Architecture

AssessGap is one Go binary (`assessgap`) with subcommands, plus a few developer tools. It reads public Cook County data into SQLite, compares a home with similar homes, and shows the result three ways: the website, the evidence report PDF and the CLI. This page is the map. [AGENTS.md](../AGENTS.md) has the rules, [methodology.md](methodology.md) the comparison method, and [data-inventory.md](data-inventory.md) the datasets and columns.

## Data flow

```
Cook County open data (Socrata)
   │  socrata: paged CSV downloads, retries
   ▼
raw_<dataset> tables ── store.LoadRaw (every column as text)
   │  ingest.Build runs build.sql
   ▼
typed tables: parcel, assessment, res_char, sale, bor_appeal, nbhd_bor_stats, …   (cook.db)
   │  comps.Engine.Report(pin, year): pool → widening search → similarity → stats → reasons
   ▼
comps.Report  ──►  web (report page, via web/views)
              ──►  packet (evidence report PDF)
              ──►  cli (check text, check -json)
```

- **Full build** (`assessgap db sync`, or `fetch`, `load`, `build`): every dataset goes to `data/raw/`, then `cook.db`. `refresh` takes a snapshot of the old and new databases and refuses a new one that shrank or lost data (`db validate`), and `db finalize` makes it ready to serve. `ops/refresh.sh` runs those steps, and `ops/deploy/sync-data.sh` ships the served tables to the server.
- **Live mode** (`check -live`, `report -live`): `live.Fetcher` downloads only one home's neighborhood, loads it with the same `store.LoadRaw` and `ingest.Build` into an in-memory SQLite database, and runs the same `comps.Engine` on it. `assessgap live-eval` checks that both paths give identical reports.
- **The website** opens `cook.db` read-only and never calls the county API. Email signups go to a separate `app.db`.

**The data seam is the typed schema in `build.sql`, read through `*sql.DB`.** The report code (`comps`) doesn't know whether the database came from a full build, live mode, the test fixture or the synthetic test builder. So there is no Go "data source" interface: the SQL schema is the interface, and a new source only has to fill the same tables.

## Packages

| Layer | Package | Job |
|---|---|---|
| Entry | `cmd/assessgap` | `main`: calls `cli.Main` |
| | `internal/cli` | Subcommands, flags, database paths, CLI text output, the evaluation commands (`backtest`, `sample`, `live-eval`) |
| Presentation | `internal/web` | HTTP server, routes, limits, the report page's view model, the chart, township pages, signups |
| | `internal/web/views` | templ templates and the view structs they render (`types.go`) |
| | `internal/packet` | The evidence report PDF (its own minimal PDF writer, `pdf.go`) |
| Engine and facts | `internal/comps` | The comparison: types (`types.go`), loading (`load.go`), selection (`select.go`), statistics and reasons (`analyze.go`), wording of differences (`describe.go`), scope (`scope.go`) |
| | `internal/bor` | Board of Review calendar and filing facts, township names, Chicago time |
| | `internal/search` | PIN parsing, address lookup |
| | `internal/format` | How figures and names read on every surface: money, rates, percents, the gap, baths, distance, addresses, PINs |
| Data | `internal/socrata` | Socrata API client |
| | `internal/store` | Opening SQLite (pragmas), loading raw CSV pages |
| | `internal/ingest` | Dataset list, the columns we keep, `build.sql` |
| | `internal/refresh` | Validation and finalizing of a rebuilt database |
| | `internal/live` | One home's data from the API, for live mode |
| Tests | `internal/fixture`, `internal/golden`, `internal/e2e` | The frozen fixture database, the golden files, and end-to-end tests of the built binary (test-only) |
| Tools | `tools/*` | Fixture rebuild, tuning, share image, PR checks (not part of the binary) |

## Dependency rules

```
cmd/assessgap ─► cli ─► web ─► web/views
                  │      └───► packet ─┐
                  │                    ├─► comps, bor, search, format   (leaves)
                  ├─► live ─► ingest ─► store ─► socrata        (leaf)
                  └─► refresh ─► ingest
```

1. **Leaves import nothing from this module:** `comps`, `bor`, `search`, `format`, `socrata`. The one exception is `search`, which uses `format` for `FormatPIN`. The engine must stay usable by any surface or tool, and `format` takes the engine's thresholds as arguments (`format.Gap(g, comps.InLine(g))`) rather than importing them.
2. **Presentation (`web`, `web/views`, `packet`) never imports the data side** (`store`, `ingest`, `refresh`, `live`, `socrata`). It reads a `comps.Report`, or the served tables through the `*sql.DB` it's given.
3. **The data side never imports presentation or `cli`.**
4. **Only `cli` imports `web`; only `web` imports `web/views`; only `cmd/assessgap` imports `cli`.** `fixture` and `golden` are imported only by tests; `e2e` holds only tests.
5. **`tools/*` may import only `comps`, `ingest` and `store`.**

`cmd/assessgap/imports_test.go` enforces these as an allow-list per package, so a new import edge or a new package fails `go test` until the list (and this page) says so.

## Where things live

| What | Where |
|---|---|
| Core types (home, comparable, statistics, report) | `comps/types.go` |
| Comparison rules: search steps, weights, how many homes | `comps/select.go` (`DefaultParams`: steps, weights, `MinComps` 8, `MaxComps` 30) |
| Thresholds: in line (±1%), groups, evidence counts, land tolerance | `comps/analyze.go` (constants at the top) |
| The rates quoted to homeowners (backtest of 2025 decisions) | `comps/analyze.go` (`groupRates`), copied into `web/views/pages.templ` and `docs/methodology.md` |
| Supported classes | `comps/scope.go` |
| Report text from the engine (reasons, flags, differences) | `comps/analyze.go`, `comps/describe.go` |
| Filing windows, deadlines, portal URLs | `bor/bor.go` (one hand-edited session a year) |
| Datasets, columns kept, year floors | `ingest/ingest.go` |
| Typed schema | `ingest/build.sql` |
| SQLite pragmas | `store/open.go` |
| Website limits and timeouts | `web/server.go`, `web/limit.go`, `cli/serve.go` |
| Formatting of money, rates, percents, addresses, PINs | `internal/format`. Each surface keeps only its own wording and its placeholder for an unknown value (`cli/format.go`, `web/report.go`, the end of `packet/packet.go`). `comps` keeps its own `money`/`pct`, which round slightly differently, for the reasons text |
| Database path rules (`-db`, `$ASSESSGAP_DB`, live by default) | `cli/source.go` |
| Goldens and fixture | `internal/golden/cases.go`, `testdata/` ([testdata/README.md](../testdata/README.md)) |

## How to add…

Counts are source files you edit, not counting generated files (`*_templ.go`, `app.css`) or goldens.

**A report field or data column** (say a new characteristic), about 9 files:
1. `ingest/ingest.go`: add the column to `Columns`. Live mode, the fixture tool and the synthetic builder all read this list.
2. `ingest/build.sql`: the typed table and its `INSERT … SELECT`. Inserts are positional, so keep the column at the same place in both.
3. `comps/types.go`, `comps/load.go`: the `Home` field and its JSON tag, and its place in `homeColumns` and `scanHome`. CLI JSON then has it.
4. `comps/describe.go`: in `Features` and the differences wording, if it describes the home.
5. `packet/packet.go`: a `columns` entry (widths must still add up to 720 pt).
6. `web/views/types.go`, `web/report.go` (`homeRow`), `web/views/report.templ`: the web grid.
7. Rebuild the fixture from a full database (`tools/fixture`) and update the goldens, explaining each change.

**Township-open emails:** signups are in `app.db` (`store/open.go` for the schema, `web/server.go` `handleNotify` for the insert). A sender needs `bor.StatusOf` for the window, a new `cli` subcommand run by a timer, unsubscribe and confirm routes in `web`, and send state in the schema. Put the signup reads and writes in their own package before a second user appears. The township name→slug helpers are in `web/township.go`.

**A nightly refresh:** the steps exist (`db sync`, `db validate`, `db finalize`, `ops/refresh.sh`, `ops/deploy/sync-data.sh`). What's missing is a scheduler entry and the hand-off from refresh to sync. The served-table list lives only in `sync-data.sh`.

**A page:** route in `web/server.go`, the template in `web/views/pages.templ` (or a new `.templ`), `indexedPaths` in `web/seo.go` for the sitemap, the header or footer in `web/views/nav.go` and `nav.templ`, and `PAGES` in `ops/stats.py`.

**Another assessment stage or year:** a new year needs a new `bor` schedule. Data years roll forward on their own. A new stage (say Board of Review final values, already in `build.sql`) touches the "certified, else mailed" choice, which is written in SQL in `comps/load.go`, `refresh/refresh.go` and `live/live.go`, plus each surface's wording (`web/report.go`, `packet/packet.go`, `cli/check.go`).

**Another county:** not planned. The seam would be the typed tables: a county fills the same schema from its own `ingest`. Cook specifics are 14-digit PINs (`search`), class codes (`comps/scope.go`), 5-digit neighborhoods and PIN blocks (`live`), and the `bor` package.
