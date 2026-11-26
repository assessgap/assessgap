#!/bin/bash
# Every page of a PDF (e.g. the evidence report) as PNG images, for a PR's
# "Visual proof".
#
#   scripts/pdf-pages.sh IN.pdf OUT_PREFIX [WIDTH [REDACT...]]
#
# Writes OUT_PREFIX-page1.png, OUT_PREFIX-page2.png, ... WIDTH is in pixels
# (default 1100). Each REDACT string (e.g. "4923 N Cali", the start of an
# address that isn't already public in testdata/golden) is covered with a
# grey box, through the end of its table cell or phrase. On macOS this uses
# PDFKit through the system swift (scripts/pdfpages.swift); elsewhere
# pdftoppm (poppler-utils), which can't redact.
set -euo pipefail

if [ $# -lt 2 ]; then
	echo "usage: $0 IN.pdf OUT_PREFIX [WIDTH [REDACT...]]" >&2
	exit 2
fi
width=${3:-1100}
mkdir -p "$(dirname "$2")"

if [ "$(uname)" = Darwin ] && command -v swift >/dev/null; then
	swift "$(dirname "$0")/pdfpages.swift" "$1" "$2" "$width" "${@:4}"
elif [ $# -gt 3 ]; then
	echo "redacting needs macOS (PDFKit)" >&2
	exit 1
elif command -v pdftoppm >/dev/null; then
	pdftoppm -png -scale-to-x "$width" -scale-to-y -1 "$1" "$2-page"
	# pdftoppm numbers pages page-1, page-01, ...; match the macOS names.
	for file in "$2"-page-*.png; do
		num=${file##*-page-}
		num=${num%.png}
		mv "$file" "$2-page$((10#$num)).png"
		echo "$2-page$((10#$num)).png"
	done
else
	echo "no PDF renderer: needs swift (macOS) or pdftoppm" >&2
	exit 1
fi
