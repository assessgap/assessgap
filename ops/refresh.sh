#!/bin/bash
# Nightly rebuild of the county database from Cook County open data.
#
#   ops/refresh.sh [-no-swap] [-resume] [-data DIR] [-pins "PIN PIN"] [-pin-prefix P] [-restart LABEL]
#
# 1. Fetch every dataset into a fresh dated dir, DATA/raw-YYYYMMDD.
#    socrata.Fetch skips pages that already exist, so an existing dir is an error
#    unless -resume is passed (continue a fetch that failed earlier the same day).
# 2. Load and build into DATA/cook.db.new. The live DATA/cook.db is never written.
# 3. Validate: integrity check, every typed table non-empty, no table shrinks >2%,
#    the latest assessment year doesn't go backwards, ./bin/check works on known PINs.
# 4. Finalize (checkpoint, rollback journal, no -wal/-shm), then swap with an atomic mv.
#    The previous database stays as DATA/cook.db.prev (hard link) for rollback.
#    -no-swap stops after finalize and leaves DATA/cook.db.new in place.
# 5. After a successful swap: restart the server (if -restart LABEL), and delete
#    raw-* dirs except the one just used.
#
# Any failed step exits nonzero; nothing after it runs and cook.db is untouched.
set -euo pipefail

# launchd starts jobs with a minimal PATH.
export PATH=/usr/local/go/bin:$HOME/go/bin:/usr/local/bin:/usr/bin:/bin

cd "$(dirname "$0")/.."

swap=1
resume=0
data=data
pins="13212230300000 10152260210000"
pin_prefix=""
restart=""

usage() {
  sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
  exit 2
}

while [ $# -gt 0 ]; do
  case "$1" in
    -no-swap) swap=0 ;;
    -resume) resume=1 ;;
    -data) data=${2:?}; shift ;;
    -pins) pins=${2:?}; shift ;;
    -pin-prefix) pin_prefix=${2:?}; shift ;;  # dev only: tiny partial builds
    -restart) restart=${2:?}; shift ;;
    -h|-help|--help) usage ;;
    *) echo "unknown argument: $1" >&2; usage ;;
  esac
  shift
done

db="$data/cook.db"
new="$db.new"
raw="$data/raw-$(date +%Y%m%d)"
lock="$data/refresh.lock"
start=$SECONDS

log() { echo "$(date '+%Y-%m-%d %H:%M:%S') $*"; }

# step NAME CMD...: run CMD, logging its duration. With set -e a failure exits.
step() {
  local name=$1 t0=$SECONDS
  shift
  log "== $name"
  "$@"
  log "== $name: done in $((SECONDS - t0))s"
}

die() { log "FAILED: $*"; exit 1; }

# One refresh at a time. The lock dir holds the owner's PID so a lock left by a
# killed run (power loss, SIGKILL) doesn't block every later night.
take_lock() {
  if mkdir "$lock" 2>/dev/null; then
    echo $$ > "$lock/pid"
    return
  fi
  local pid
  pid=$(cat "$lock/pid" 2>/dev/null || true)
  if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
    die "another refresh is running (pid $pid)"
  fi
  log "removing stale lock (pid ${pid:-unknown} is gone)"
  rm -rf "$lock"
  mkdir "$lock" || die "could not take lock $lock"
  echo $$ > "$lock/pid"
}
take_lock
trap 'rm -rf "$lock"' EXIT
# Make INT/TERM (e.g. launchctl stop) exit through the EXIT trap.
trap 'exit 130' INT
trap 'exit 143' TERM

ingest_flags=(-raw "$raw")
if [ -n "$pin_prefix" ]; then
  ingest_flags+=(-pin-prefix "$pin_prefix")
fi

if [ -e "$raw" ] && [ "$resume" -eq 0 ]; then
  die "$raw already exists; pass -resume to continue today's fetch, or remove it"
fi

log "refresh: data=$data raw=$raw swap=$swap"

step "build binaries" go build -o bin/ ./cmd/ingest ./cmd/build ./cmd/check ./cmd/refresh

# A leftover from a failed run must not leak old tables into this build.
rm -f "$new" "$new-wal" "$new-shm"

step "fetch" ./bin/ingest "${ingest_flags[@]}" fetch
step "load" ./bin/ingest "${ingest_flags[@]}" -db "$new" load
step "build" ./bin/build -db "$new"
step "validate" ./bin/refresh validate -old "$db" -new "$new"

check_pins() {
  local pin out verdict
  for pin in $pins; do
    out=$(./bin/check -db "$new" -brief "$pin")
    case "$out" in
      *'"verdict": "'*) ;;
      *) die "check $pin: report has no verdict" ;;
    esac
    verdict=${out#*\"verdict\": \"}
    log "check $pin: verdict ${verdict%%\"*}"
  done
}
step "check known PINs" check_pins

finalize() {
  ./bin/refresh finalize -db "$new"
  rm -f "$new-wal" "$new-shm"
}
step "finalize" finalize

if [ "$swap" -eq 0 ]; then
  log "-no-swap: leaving $new in place ($(du -h "$new" | cut -f1)); total $((SECONDS - start))s"
  exit 0
fi

swap_db() {
  # The server only reads cook.db, so its WAL should be empty. A non-empty WAL
  # would hold pages that belong to the old file; refuse rather than orphan them.
  if [ -s "$db-wal" ]; then
    die "$db-wal is not empty; not swapping"
  fi
  if [ -e "$db" ]; then
    ln -f "$db" "$db.prev"
  fi
  mv -f "$new" "$db"
  rm -f "$db-wal" "$db-shm"
}
step "swap" swap_db

if [ -n "$restart" ]; then
  step "restart server" launchctl kickstart -k "gui/$(id -u)/$restart"
else
  log "no -restart label: restart the server so every connection reads the new $db"
fi

prune_raw() {
  local d
  for d in "$data"/raw-*; do
    if [ -d "$d" ] && [ "$d" != "$raw" ]; then
      log "removing $d"
      rm -rf "$d"
    fi
  done
}
step "prune raw dirs" prune_raw

log "refresh complete in $((SECONDS - start))s"
