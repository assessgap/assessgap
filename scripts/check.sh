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

# Package directories tracked by git, as ./relative paths (local data
# directories such as data-*/ are git-ignored and may hold scratch Go code).
packages() {
	# -e: broken scratch code in an ignored directory must not stop the listing.
	go list -e -f '{{.Dir}}' ./... | while read -r dir; do
		if [ "$dir" = "$PWD" ]; then
			echo .
		else
			git check-ignore -q "$dir" || echo "./${dir#"$PWD"/}"
		fi
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
	# shellcheck disable=SC2086 # $pkgs is a list of import paths
	if [ -n "$bin" ] && "$bin" version 2>/dev/null | grep -Eq "version v?${GOLANGCI_LINT_VERSION#v} "; then
		"$bin" run $pkgs
	else
		go run "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$GOLANGCI_LINT_VERSION" run $pkgs
	fi
}

# Regenerate templ and Tailwind output and fail if anything changed or
# appeared. The regenerated files are left in place to review and commit.
# (step runs this inside "if !", where set -e is off, hence the || return 1.)
check_generated() {
	local files before after new
	files=$(git ls-files 'internal/web/views/*_templ.go' internal/web/static/app.css)
	before=$(git hash-object $files) || return 1
	go tool templ generate -path internal/web/views || return 1
	if [ -x bin/tailwindcss ]; then
		(cd internal/web && ../../bin/tailwindcss -i styles/input.css -o static/app.css --minify) || return 1
	fi
	after=$(git hash-object $files) || return 1
	new=$(git status --porcelain --untracked-files=all -- internal/web/views internal/web/static | grep '^??' || true)
	if [ "$before" != "$after" ] || [ -n "$new" ]; then
		echo "generated files were stale or missing and have been regenerated; review and commit:"
		git status --short --untracked-files=all -- internal/web/views internal/web/static
		return 1
	fi
}

if [ ! -x bin/tailwindcss ]; then
	if [ "${REQUIRE_TAILWIND:-}" = 1 ]; then
		scripts/tailwind.sh >/dev/null
	else
		echo "note: no bin/tailwindcss (run scripts/tailwind.sh); app.css is not checked" >&2
	fi
fi

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
