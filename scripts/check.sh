#!/bin/bash
# The gate before pushing: the same steps CI runs. Prints only failures.
#
#   scripts/check.sh
#
# Needs Go (the version in go.mod). golangci-lint is used from PATH if it is
# the pinned version, otherwise run through "go run" (downloaded once, then
# cached). Tailwind: bin/tailwindcss from scripts/tailwind.sh. Without it the
# app.css check is skipped with a note; REQUIRE_TAILWIND=1 (CI) downloads it
# instead, and fails if it can't.
set -euo pipefail
cd "$(dirname "$0")/.."

GOLANGCI_LINT_VERSION=v2.15.0

failed=0
out=$(mktemp)
trap 'rm -f "$out"' EXIT

# step NAME CMD...: run CMD quietly; on failure print NAME and its output.
step() {
	local name=$1
	shift
	if ! "$@" >"$out" 2>&1; then
		echo "FAIL: $name"
		sed 's/^/  /' "$out"
		failed=1
	fi
}

# Packages tracked by git (local data directories such as data-*/ are
# git-ignored and may hold scratch Go code).
packages() {
	go list -f '{{.ImportPath}} {{.Dir}}' ./... | while read -r pkg dir; do
		git check-ignore -q "$dir" || echo "$pkg"
	done
}

check_gofmt() {
	local files
	files=$(git ls-files '*.go' | grep -v '_templ\.go$' | xargs gofmt -l)
	if [ -n "$files" ]; then
		echo "not gofmt-ed (run gofmt -w):"
		echo "$files"
		return 1
	fi
}

# Vague package and file names hide what code does and attract unrelated code.
check_names() {
	local bad
	bad=$(git ls-files | grep -E '(^|/)(util|utils|helper|helpers|common|misc)(/|\.go$|_test\.go$)' || true)
	if [ -n "$bad" ]; then
		echo "vague package or file names; name them for what they do:"
		echo "$bad"
		return 1
	fi
}

lint() {
	local bin
	bin=$(command -v golangci-lint || true)
	if [ -n "$bin" ] && "$bin" version 2>/dev/null | grep -q "version ${GOLANGCI_LINT_VERSION#v} "; then
		"$bin" run ./...
	else
		go run "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$GOLANGCI_LINT_VERSION" run ./...
	fi
}

# Regenerate templ and Tailwind output and fail if anything changed. The
# regenerated files are left in place for you to review and commit.
check_generated() {
	local files before after
	files=$(git ls-files 'internal/web/views/*_templ.go' internal/web/static/app.css)
	before=$(git hash-object $files)
	go tool templ generate -path internal/web/views
	if [ ! -x bin/tailwindcss ] && [ "${REQUIRE_TAILWIND:-}" = 1 ]; then
		scripts/tailwind.sh
	fi
	if [ -x bin/tailwindcss ]; then
		(cd internal/web && ../../bin/tailwindcss -i styles/input.css -o static/app.css --minify)
	else
		echo "note: no bin/tailwindcss (run scripts/tailwind.sh); app.css not checked" >&2
	fi
	after=$(git hash-object $files)
	if [ "$before" != "$after" ]; then
		echo "generated files were stale and have been regenerated; review and commit:"
		git status --short $files
		return 1
	fi
}

pkgs=$(packages)

step "gofmt" check_gofmt
step "package and file names" check_names
# shellcheck disable=SC2086 # $pkgs is a list of import paths
step "go vet" go vet $pkgs
step "golangci-lint $GOLANGCI_LINT_VERSION" lint
# shellcheck disable=SC2086
step "go test" go test $pkgs
step "generated files up to date" check_generated
# shellcheck disable=SC2086
step "go build" go build $pkgs

exit $failed
