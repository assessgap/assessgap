#!/bin/bash
# Desktop and phone screenshots of site pages, for a PR's "Visual proof".
#
#   scripts/screenshots.sh [-redact REGEX] BASE_URL OUT_DIR PAGE...
#   scripts/screenshots.sh http://127.0.0.1:8091 shots/after / /townships \
#       'report-grid=/check/13124210120000@844!.overflow-x-auto'
#
# -redact REGEX replaces every match of the JavaScript regular expression in
# the page's text, inputs and title with "[hidden]" before capturing, e.g.
# -redact '\d+ [NSEW] [A-Za-z]+( (Ave|St))?' for the street addresses of
# homes that aren't already public in testdata/golden.
#
# PAGE is [name=]path[@height][!selector]:
#   name      file name stem; defaults to the path with "/" turned into "-"
#             ("/" is "home")
#   @height   capture only the first HEIGHT pixels (one screen); without it
#             the whole page is captured
#   !selector before capturing, scroll the first element matching this CSS
#             selector to the top of the screen and all the way to its right
#             end (shows a sideways-scrolling table); implies @844
# Each page gives OUT_DIR/<name>-desktop<SUFFIX>.png (1280 wide) and
# OUT_DIR/<name>-mobile<SUFFIX>.png (390 wide, mobile emulation), and warns
# if the page is wider than the screen.
#
# Headless Chrome's own --window-size can't go below 500 px, so this drives
# Chrome through its DevTools protocol on a pipe (bash and Chrome only).
# Environment: SUFFIX (e.g. -before); CHROME, the browser binary (default
# Google Chrome on macOS, else google-chrome, chromium or chromium-browser).
# Never commit the images to a code branch; see "Visual proof" in
# CONTRIBUTING.md.
set -euo pipefail

redact=
if [ "${1:-}" = -redact ] && [ $# -ge 2 ]; then
	redact=$2
	shift 2
fi
if [ $# -lt 3 ]; then
	echo "usage: $0 [-redact REGEX] BASE_URL OUT_DIR PAGE..." >&2
	exit 2
fi
base=${1%/}
out=$2
shift 2
mkdir -p "$out"

find_chrome() {
	local mac="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" name
	if [ -n "${CHROME:-}" ]; then
		echo "$CHROME"
	elif [ -x "$mac" ]; then
		echo "$mac"
	else
		for name in google-chrome google-chrome-stable chromium chromium-browser; do
			if command -v "$name" >/dev/null; then
				command -v "$name"
				return
			fi
		done
		echo "no Chrome or Chromium found; set CHROME" >&2
		return 1
	fi
}
chrome=$(find_chrome)

# A throwaway profile and the two pipes Chrome talks on: it reads commands
# on fd 3 and writes replies on fd 4, each a JSON message ending in a NUL.
tmp=$(mktemp -d)
pid=
cleanup() {
	if [ -n "$pid" ]; then
		kill "$pid" 2>/dev/null || true
		wait "$pid" 2>/dev/null || true
	fi
	rm -rf "$tmp"
}
trap cleanup EXIT
mkfifo "$tmp/in" "$tmp/out"
"$chrome" --headless=new --disable-gpu --hide-scrollbars --no-first-run \
	--no-default-browser-check --user-data-dir="$tmp/profile" \
	--remote-debugging-pipe about:blank 3<"$tmp/in" 4>"$tmp/out" 2>/dev/null &
pid=$!
exec 5>"$tmp/in" 6<"$tmp/out"

id=0
session=
reply=
# Set when the page's load event arrives, which can be before the reply to
# Page.navigate.
loaded=0

# next: read Chrome's next message into $reply, noting a load event; fail
# after 60 s rather than hang.
next() {
	if ! IFS= read -r -t 60 -d '' reply <&6; then
		echo "FAIL: no reply from Chrome" >&2
		return 1
	fi
	case $reply in
	*'"method":"Page.loadEventFired"'*) loaded=1 ;;
	esac
}

# call METHOD PARAMS: send a command to the page and wait for its reply.
call() {
	id=$((id + 1))
	local to=
	if [ -n "$session" ]; then
		to=",\"sessionId\":\"$session\""
	fi
	printf '%s\0' "{\"id\":$id,\"method\":\"$1\",\"params\":$2$to}" >&5
	while :; do
		next || return 1
		case $reply in
		"{\"id\":$id,"*) break ;;
		esac
	done
	case $reply in
	*'"error":'*)
		echo "FAIL: $1: $reply" >&2
		return 1
		;;
	esac
}

# wait_loaded: wait for the page's load event.
wait_loaded() {
	while [ "$loaded" = 0 ]; do
		next || return 1
	done
}

# field NAME: the value of "NAME": in the last reply (a string or a number).
field() {
	local rest=${reply#*\"$1\":}
	rest=${rest#\"}
	echo "${rest%%[\",\}]*}"
}

# eval_js EXPR: evaluate EXPR in the page and print its value.
eval_js() {
	local expr=${1//\\/\\\\}
	expr=${expr//\"/\\\"}
	expr=${expr//$'\n'/ }
	expr=${expr//$'\t'/ }
	call Runtime.evaluate "{\"expression\":\"$expr\",\"awaitPromise\":true,\"returnByValue\":true}"
	field value
}

call Target.createTarget '{"url":"about:blank"}'
target=$(field targetId)
call Target.attachToTarget "{\"targetId\":\"$target\",\"flatten\":true}"
session=$(field sessionId)
call Page.enable '{}'

# shoot FILE WIDTH MOBILE URL HEIGHT SELECTOR
shoot() {
	local file=$1 width=$2 mobile=$3 url=$4 height=$5 selector=$6 full wide
	call Emulation.setDeviceMetricsOverride \
		"{\"width\":$width,\"height\":${height:-844},\"deviceScaleFactor\":1,\"mobile\":$mobile}"
	loaded=0
	call Page.navigate "{\"url\":\"$url\"}"
	case $reply in
	*'"errorText"'*)
		echo "FAIL: $url: $(field errorText)" >&2
		return 1
		;;
	esac
	wait_loaded
	eval_js 'document.fonts.ready.then(() => 1)' >/dev/null
	if [ -n "$redact" ]; then
		local pattern=${redact//\\/\\\\}
		pattern=${pattern//\'/\\\'}
		eval_js "(() => { const re = new RegExp('$pattern', 'g'), hide = '[hidden]';
			const walk = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
			while (walk.nextNode()) { const t = walk.currentNode, v = t.nodeValue.replace(re, hide); if (v !== t.nodeValue) t.nodeValue = v; }
			document.querySelectorAll('input').forEach(i => { i.value = i.value.replace(re, hide); });
			document.title = document.title.replace(re, hide); return 1; })()" >/dev/null
	fi
	if [ -n "$selector" ]; then
		selector=${selector//\'/\\\'}
		eval_js "(() => { const el = document.querySelector('$selector'); if (!el) return 0; el.scrollIntoView(); el.scrollLeft = el.scrollWidth;
			const top = document.querySelector('header'), pos = top ? getComputedStyle(top).position : '';
			window.scrollBy(0, -((pos === 'sticky' || pos === 'fixed' ? top.offsetHeight : 0) + 8)); return 1; })()" |
			grep -q 1 || echo "warning: no element matches $selector on $url" >&2
	fi
	wide=$(eval_js 'document.documentElement.scrollWidth')
	if [ "$wide" -gt "$width" ]; then
		echo "warning: $url is $wide px wide at a $width px screen" >&2
	fi
	if [ -z "$height" ]; then
		full=$(eval_js 'Math.ceil(document.documentElement.scrollHeight)')
		if [ "$full" -gt 16000 ]; then
			full=16000
		fi
		call Emulation.setDeviceMetricsOverride \
			"{\"width\":$width,\"height\":$full,\"deviceScaleFactor\":1,\"mobile\":$mobile}"
	fi
	call Page.captureScreenshot '{"format":"png"}'
	# sed, not field: bash's ${var#*pattern} is quadratic on megabytes.
	printf '%s' "$reply" | sed 's/.*"data":"\([^"]*\)".*/\1/' | base64 --decode >"$file"
	echo "$file"
}

for page in "$@"; do
	height= selector= name=
	case $page in
	*!*)
		selector=${page#*!}
		page=${page%%!*}
		height=844
		;;
	esac
	case $page in
	*@*)
		height=${page##*@}
		page=${page%@*}
		;;
	esac
	# A name has no "/" (so a query string's "=" is not mistaken for one).
	case ${page%%=*} in
	*/*) ;;
	*)
		if [ "${page%%=*}" != "$page" ]; then
			name=${page%%=*}
			page=${page#*=}
		fi
		;;
	esac
	if [ -z "$name" ]; then
		name=${page%%#*}
		name=${name#/}
		name=${name%/}
		name=${name//\//-}
		name=${name//\?/-}
		name=${name:-home}
	fi
	shoot "$out/$name-desktop${SUFFIX:-}.png" 1280 false "$base$page" "$height" "$selector"
	shoot "$out/$name-mobile${SUFFIX:-}.png" 390 true "$base$page" "$height" "$selector"
done

session=
call Browser.close '{}' || true
