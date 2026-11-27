#!/bin/bash
# Read-only check of a running AssessGap site after a deploy: the key routes,
# their status codes and content types, and one public golden home's figures.
#
#   scripts/smoke.sh [base-url]        # default https://assessgap.com
#   scripts/smoke.sh http://127.0.0.1:8121
#
# About 12 sequential GETs, nothing else (no POSTs, no load). Exits 0 when
# every check passes, 1 otherwise, printing one line per check.
#
# Figures: the report page for PIN 13331030130000 (public in testdata/golden)
# must show the building AV per sq ft for the home and the median. Against a
# local server on the frozen fixture they must equal the golden exactly.
# Against any other site they are compared with the golden too, but a
# difference is a warning unless SMOKE_STRICT=1, because a live site's
# database is rebuilt from current county data and the golden is frozen.
set -euo pipefail
cd "$(dirname "$0")/.."

base=${1:-https://assessgap.com}
base=${base%/}
pin=13331030130000
golden=testdata/golden/web/well-above-mailed.txt

strict=${SMOKE_STRICT:-0}
case "$base" in
  http://127.0.0.1:* | http://localhost:*) strict=1 ;;
esac

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
failed=0

ok() { echo "ok    $*"; }
bad() { echo "FAIL  $*"; failed=1; }
warn() { echo "warn  $*"; }

# get PATH: fetch base+PATH without following redirects into $tmp/body and
# $tmp/headers; sets status, ctype and location.
get() {
  : >"$tmp/headers"
  status=$(curl -sS --compressed --max-time 30 -A 'assessgap-smoke' \
    -o "$tmp/body" -D "$tmp/headers" -w '%{http_code}' "$base$1") || status=000
  ctype=$(header Content-Type)
  location=$(header Location)
}

# header NAME: the value of a response header from the last get.
header() {
  local want
  want=$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')
  tr -d '\r' <"$tmp/headers" | while IFS= read -r line; do
    name=$(printf '%s' "${line%%:*}" | tr '[:upper:]' '[:lower:]')
    if [ "$name" = "$want" ]; then
      printf '%s\n' "${line#*: }"
    fi
  done | tail -1
}

# expect PATH STATUS CONTENT-TYPE-PREFIX: one route's status and type.
expect() {
  get "$1"
  if [ "$status" != "$2" ]; then
    bad "$1: status $status, want $2"
    return 1
  fi
  case "$ctype" in
    "$3"*) ok "$1: $status $ctype" ;;
    *) bad "$1: Content-Type \"$ctype\", want $3"; return 1 ;;
  esac
}

# expect_redirect PATH STATUS LOCATION
expect_redirect() {
  get "$1"
  if [ "$status" = "$2" ] && [ "$location" = "$3" ]; then
    ok "$1: $status -> $location"
  else
    bad "$1: $status -> \"$location\", want $2 -> $3"
  fi
}

echo "smoke: $base"

if expect /healthz 200 text/plain; then
  [ "$(cat "$tmp/body")" = ok ] || bad "/healthz: body \"$(head -c 40 "$tmp/body")\", want ok"
fi

if expect / 200 text/html; then
  case "$(header Content-Security-Policy)" in
    "default-src 'self';"*) ok "/: same-origin Content-Security-Policy" ;;
    *) bad "/: Content-Security-Policy \"$(header Content-Security-Policy)\"" ;;
  esac
  if [ -n "$(header X-Robots-Tag)" ] && [ "$strict" != 1 ]; then
    bad "/: X-Robots-Tag \"$(header X-Robots-Tag)\" (serve without -dev=false?)"
  fi
  css=$(grep -o '/static/app\.css?v=[0-9a-f]*' "$tmp/body" | head -1 || true)
  if [ -z "$css" ]; then
    bad "/: links no versioned /static/app.css"
  elif expect "$css" 200 text/css; then
    case "$(header Cache-Control)" in
      *immutable*) ok "$css: cached as immutable" ;;
      *) bad "$css: Cache-Control \"$(header Cache-Control)\"" ;;
    esac
  fi
fi

expect /townships 200 text/html || true
expect /townships/jefferson 200 text/html || true
expect_redirect /townships/ 301 /townships
expect_redirect "/search?q=$pin" 303 "/check/$pin"

if expect /robots.txt 200 text/plain; then
  grep -q '^Sitemap: ' "$tmp/body" || bad "/robots.txt: no Sitemap line (dev mode disallows everything)"
fi
if expect /sitemap.xml 200 application/xml; then
  n=$(grep -o '<loc>' "$tmp/body" | wc -l | tr -d ' ')
  [ "$n" -gt 0 ] && ok "/sitemap.xml: $n URLs" || bad "/sitemap.xml: no <loc> entries"
fi

if expect "/check/$pin" 200 text/html; then
  label=$(grep -o 'aria-label="Your home \$[0-9.,]*/sq ft; Median \$[0-9.,]*/sq ft"' "$tmp/body" | head -1 || true)
  if [ -z "$label" ]; then
    bad "/check/$pin: no \"Your home \$X/sq ft; Median \$Y/sq ft\" figures"
  else
    got=${label#aria-label=\"}
    got=${got%\"}
    want="$(sed -n 's/^ChartLabels.Subject = "\(.*\)"$/\1/p' "$golden"); $(sed -n 's/^ChartLabels.Median = "\(.*\)"$/\1/p' "$golden")"
    if [ "$got" = "$want" ]; then
      ok "/check/$pin: $got (matches $golden)"
    elif [ "$strict" = 1 ]; then
      bad "/check/$pin: \"$got\", golden \"$want\""
    else
      warn "/check/$pin: \"$got\", golden \"$want\" (county data may have changed since the fixture)"
    fi
  fi
fi

if expect "/check/$pin/report.pdf" 200 application/pdf; then
  if [ "$(head -c 7 "$tmp/body")" = "%PDF-1." ] && tail -c 32 "$tmp/body" | grep -q '%%EOF'; then
    ok "/check/$pin/report.pdf: $(wc -c <"$tmp/body" | tr -d ' ') bytes, whole PDF"
  else
    bad "/check/$pin/report.pdf: not a whole PDF"
  fi
fi

expect /no-such-page 404 text/html || true

if [ "$failed" -ne 0 ]; then
  echo "smoke: FAILED"
  exit 1
fi
echo "smoke: ok"
