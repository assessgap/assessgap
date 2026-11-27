# Workflows waiting to be installed

The automation that opened this change can't write to `.github/workflows/`, so new workflows wait here for a maintainer.

## `release.yml`

Builds `assessgap` for macOS, Linux and Windows (amd64 and arm64) with GoReleaser, using the repository's `.goreleaser.yaml`. It attaches the archives (each with `LICENSE` and `README.md`) and `checksums.txt` to a **draft** GitHub release, with a changelog made from the commits since the last tag. It runs only on pushed tags matching `v*`, and its token can only write repository contents (`contents: write`).

**Install:**

```sh
git mv .github/workflows-pending/release.yml .github/workflows/release.yml
```

Then commit through a pull request as usual.

**Optional hardening:** the actions are pinned to exact release tags. To pin them to commit SHAs instead, look up each tag's commit and replace `@vX.Y.Z` with `@<sha> # vX.Y.Z`:

```sh
git ls-remote https://github.com/actions/checkout refs/tags/v4.2.2
git ls-remote https://github.com/actions/setup-go refs/tags/v5.5.0
git ls-remote https://github.com/goreleaser/goreleaser-action refs/tags/v6.3.0
```

**Make a release** (the maintainer's call):

```sh
go run github.com/goreleaser/goreleaser/v2@v2.20.3 check                       # config is valid
go run github.com/goreleaser/goreleaser/v2@v2.20.3 release --snapshot --clean  # local test build into dist/
git tag v0.1.0 && git push origin v0.1.0                                       # the workflow builds the draft release
```

Review the draft on GitHub, then publish it.

## `fuzz.yml`

Runs each fuzz target (`*_fuzz_test.go`: PIN parsing, the address normalizer, township slugs and redirects, the search and report routes, live mode's handling of county CSV and SoQL quoting, Socrata page fetching, and `build.sql`'s field parsing) for 3 minutes, nightly and on demand. Their seed corpora already run in `scripts/check.sh` as ordinary tests, so this job only adds the search for new inputs; it never blocks a pull request. On a failure it uploads the input as an artifact: reproduce with `go test ./<pkg> -run <Target>/<file>`, fix, and commit the file under `testdata/fuzz/<Target>/` as the regression seed. Read-only token (`contents: read`). Why each target exists, and what the first runs found: `docs/dev/2026-11-27-e2e.md`.

**Install:**

```sh
git mv .github/workflows-pending/fuzz.yml .github/workflows/fuzz.yml
```
