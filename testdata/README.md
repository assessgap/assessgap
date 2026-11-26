# Test data

Two things live here: a frozen slice of real county data (`fixture/`) and the golden outputs computed from it (`golden/`). Together they make every homeowner-facing number reviewable: if a change moves a number, a file under `golden/` changes in the same pull request.

## `fixture/`: frozen county data

One CSV page per ingested dataset, with only the columns `build.sql` reads (`ingest.Columns`). Tests build it into SQLite with the same code as a real build (`ingest.Load`, then `ingest.Build`) through `internal/fixture`, so CI runs every test that needs a database, and the fixture exercises `build.sql` too.

| | |
|---|---|
| Neighborhoods | Assessor neighborhoods **27080** (Oak Park Township; 2026 values certified), **39082** (Worth Township; certified) and **71280** (Jefferson Township, Chicago; 2026 values mailed, not yet certified) |
| Rows | every row, in every dataset, for every PIN whose parcel or assessed-value record is in those neighborhoods: 3,851 parcels, 19,251 assessed values (2022–2026), 6,054 characteristic cards, 7,702 addresses, 1,510 sales, 604 condo cards, 3,416 Board of Review appeals (2021–2025), 3,181 Assessor appeals |
| Size | 3.8 MB of plain, diffable CSV |
| Taken from | a full local build (`assessgap db sync`) whose downloads were made on 2026-11-24. Rebuilt on 2026-11-25 from the same download to add the assessed values' building and land columns (`*_bldg`, `*_land`); no existing value changed |
| Why these three | three townships with different AV stages (mailed vs certified), all three class groups, condos, two-flats, multi-building parcels, commercial and exempt parcels, and real Board of Review appeal history (288 to 977 appeals per neighborhood in 2023–2025) |

**Provenance.** Everything here is public data from the Cook County open data portal ([datacatalog.cookcountyil.gov](https://datacatalog.cookcountyil.gov/)), published by the Cook County Assessor's Office and the Board of Review:

| Directory | Dataset |
|---|---|
| `parcel_universe` | Assessor - Parcel Universe (Current) (`pabr-t5kh`) |
| `addresses` | Assessor - Parcel Addresses (`3723-97qp`) |
| `assessed_values` | Assessor - Assessed Values (`uzyt-m557`) |
| `sales` | Assessor - Parcel Sales (`wvhk-k5uv`) |
| `res_chars` | Assessor - Residential Single and Multi-Family Improvement Characteristics (`x54s-btds`) |
| `condo_chars` | Assessor - Residential Condominium Unit Characteristics (`3r7i-mrz4`) |
| `bor_appeals` | Board of Review Appeal Decision History (`7pny-nedm`) |
| `ccao_appeals` | Assessor - Appeals (`y282-6ig3`) |

**Terms.** The portal's dataset metadata names no licence (`license: null` for these datasets when checked on 2026-11-25); the data is published for public use under the portal's terms of use. Cite "Cook County Assessor's Office / Cook County Board of Review, via the Cook County open data portal".

**Nothing personal.** The fixture holds parcel records only: PINs, property addresses, classes, assessed values, building characteristics, recorded sale prices and appeal outcomes. It has no owner or taxpayer names. The one column that names people, the Assessor appeals' `agent_name` (attorneys and agents), is replaced with `AGENT` by the builder; nothing reads it.

**Rebuild** (deliberately; the fixture never changes when the county's data does):

```sh
go run ./tools/fixture -src path/to/full/cook.db -out testdata/fixture -nbhd 27080,39082,71280
go test ./internal/golden ./internal/web -run Golden -update
```

The builder opens the source read-only and writes rows sorted by every column, so the same source gives byte-identical files. A rebuild **must** update the goldens in the same pull request, and the description must say why the data changed.

To poke at the fixture by hand, build it into a scratch database (any `data-*/` directory is git-ignored):

```sh
go run ./cmd/assessgap db load -raw testdata/fixture -db data-fixture/cook.db
go run ./cmd/assessgap db build -db data-fixture/cook.db
go run ./cmd/assessgap check -db data-fixture/cook.db 13331030130000
```

## `golden/`: expected outputs

| Directory | What | Written by |
|---|---|---|
| `check/` | `assessgap check -json` for 32 cases (exit status, stderr, and every field of the report) | `internal/golden` |
| `pdf/` | the evidence report's text, for 5 PINs, prepared on a fixed date | `internal/golden` |
| `web/` | the report page's formatted figures (`web.toView`) for 8 cases, plus the township pages and the townships index on fixed dates; the date-dependent filing window and the chart geometry are left out | `internal/web` |

Structured output is flattened to one `path = value` line per field, sorted, with floats rounded to 6 decimal places, so a diff names the field that changed. On a mismatch the test prints `case (PIN): field: old → new`.

The cases are listed, with the reason each was chosen, in `internal/golden/cases.go`. They cover: well above, slightly above, in line with and below the median; above but with few comparable homes; 1 and 4 comparable homes; every search widening level; one-story, multi-story, split-level and townhome classes; ties; each data check the fixture can reach (square footage, no full baths, more bedrooms than rooms, a land rate unlike similar homes'); a big lot whose total AV per sq ft is high but whose building is assessed below the median; recorded sales; no address; an earlier certified year; a PIN whose latest value is from 2023; condo, multi-building, two-flat, exempt and class 241 parcels; an unknown PIN; and a known PIN asked for a missing year.

Edge cases real data doesn't reach cleanly (0 or 2 comparable homes, an even-count median with exact hand-worked values, zero square feet, no characteristics row, unknown year built, no building/land split or a $0 building, unknown location) are tested with hand-written homes in `internal/comps/synthetic_test.go`, not here.

**Updating.** Only after a deliberate change:

```sh
go test ./internal/golden ./internal/web -run Golden -update
git diff testdata/golden
```

The `git diff` is the review artifact. Explain every changed number in the pull request's "Numbers impact" section.
