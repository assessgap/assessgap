#!/bin/bash
# Run on the machine that holds the full county DB: build the slim serving DB from data/cook.db and ship it to the droplet.
#   ops/deploy/sync-data.sh
set -euo pipefail
cd "$(dirname "$0")/../.."
host=${DEPLOY_HOST:-assessgap-web}
tables="parcel assessment res_char sale nbhd_bor_stats"
in=('"parcel"' '"assessment"' '"res_char"' '"sale"' '"nbhd_bor_stats"')
list=$(IFS=,; echo "${in[*]}")
rm -f data/serve.db.tmp
sqlite3 data/cook.db ".dump $tables" | sqlite3 data/serve.db.tmp
# .dump with table names omits indexes; copy them explicitly.
sqlite3 data/cook.db "select sql||';' from sqlite_master where type='index' and sql is not null and tbl_name in ($list)" \
  | sqlite3 data/serve.db.tmp
sqlite3 data/serve.db.tmp "ANALYZE;" "VACUUM;"
[ "$(sqlite3 data/serve.db.tmp 'PRAGMA integrity_check')" = ok ]
[ "$(sqlite3 data/serve.db.tmp "select count(*) from sqlite_master where type='index'")" -ge 5 ]
mv data/serve.db.tmp data/serve.db
rsync -a --partial --info=progress2 data/serve.db "$host:/tmp/cook.db.new"
ssh "$host" 'set -euo pipefail
  sudo install -m 644 -o assessgap -g assessgap /tmp/cook.db.new /srv/assessgap/data/cook.db.new
  sudo mv /srv/assessgap/data/cook.db.new /srv/assessgap/data/cook.db
  rm /tmp/cook.db.new
  sudo systemctl restart assessgap
  sleep 1; curl -fsS --retry 15 --retry-connrefused --retry-delay 1 -o /dev/null -w "healthz %{http_code}\n" http://127.0.0.1:8080/healthz'
