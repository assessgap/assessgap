# Workflows waiting to be installed

These belong in `.github/workflows/`. They are here only because the session that wrote them could not push workflow files: its GitHub credential lacks the `workflow` scope. Nothing in this directory runs.

To install them, from a checkout with that permission:

```sh
git mv -f .github/workflows-pending/ci.yml .github/workflows/ci.yml
git mv .github/workflows-pending/pr-loc.yml .github/workflows-pending/pr-description.yml .github/workflows/
git rm .github/workflows-pending/README.md
```

| File | What |
|---|---|
| `ci.yml` | Replaces the current CI: runs `scripts/check.sh` (gofmt, names, vet, golangci-lint v2.15.0 installed with `go install`, tests including goldens, generated files, build) with `REQUIRE_TAILWIND=1` |
| `pr-loc.yml` | Writes the test vs production line-count table into each PR's description (`tools/prloc`) |
| `pr-description.yml` | Fails a PR whose "Numbers impact" or "Risk and rollback" is empty, or says "None" while goldens changed (`tools/prcheck`) |

The two PR workflows use `pull_request_target` and never check out PR code; see the comments at the top of each.
