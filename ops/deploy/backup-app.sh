#!/bin/bash
# Run on a machine you control (cron, nightly): copy the droplet's app.db (email signups)
# to data/backups/app-YYYYMMDD.db using SQLite's online backup. Keeps 30 days.
# If BACKUP_PING_URL is set (e.g. a Healthchecks.io check), it is pinged on success
# and <url>/fail on failure, so a missed or failed night sends an alert.
set -euo pipefail
ping() { [ -n "${BACKUP_PING_URL:-}" ] && curl -fsS -m 10 --retry 3 -o /dev/null "$BACKUP_PING_URL$1" || true; }
trap 'ping /fail' ERR
cd "$(dirname "$0")/../.."
host=${DEPLOY_HOST:-assessgap-web}
day=$(date +%Y%m%d)
mkdir -p data/backups
# A copy left by an interrupted run is owned by deploy, so assessgap couldn't
# overwrite it and every later night would fail: remove it first.
ssh "$host" 'set -euo pipefail
  sudo rm -f /tmp/app-backup.db
  sudo -u assessgap sqlite3 /srv/assessgap/data/app.db ".backup /tmp/app-backup.db"
  sudo chown deploy /tmp/app-backup.db'
scp -q "$host:/tmp/app-backup.db" "data/backups/app-$day.db"
ssh "$host" 'rm /tmp/app-backup.db'
[ "$(sqlite3 "data/backups/app-$day.db" 'PRAGMA integrity_check')" = ok ]
find data/backups -name 'app-*.db' -mtime +30 -delete
echo "backup ok: data/backups/app-$day.db ($(sqlite3 "data/backups/app-$day.db" 'select count(*) from signup') signups)"
ping ""
