#!/bin/bash
# Run on a machine you control (cron, nightly): copy the droplet's app.db (signups, later orders)
# to data/backups/app-YYYYMMDD.db using SQLite's online backup. Keeps 30 days.
set -euo pipefail
cd "$(dirname "$0")/../.."
host=${DEPLOY_HOST:-assessgap-web}
day=$(date +%Y%m%d)
mkdir -p data/backups
ssh "$host" 'set -euo pipefail
  sudo -u assessgap sqlite3 /srv/assessgap/data/app.db ".backup /tmp/app-backup.db"
  sudo chown deploy /tmp/app-backup.db'
scp -q "$host:/tmp/app-backup.db" "data/backups/app-$day.db"
ssh "$host" 'rm /tmp/app-backup.db'
[ "$(sqlite3 "data/backups/app-$day.db" 'PRAGMA integrity_check')" = ok ]
find data/backups -name 'app-*.db' -mtime +30 -delete
echo "backup ok: data/backups/app-$day.db ($(sqlite3 "data/backups/app-$day.db" 'select count(*) from signup') signups)"
