#!/bin/bash
# Live logs from the web server.
#   ops/logs.sh            app log (follow)
#   ops/logs.sh access     requests as they arrive: time, status, method, path, ms (follow)
#   ops/logs.sh errors     app warnings/errors and 5xx responses from the last day
set -euo pipefail
host=${DEPLOY_HOST:-assessgap-web}
case "${1:-app}" in
app) ssh -t "$host" 'sudo journalctl -u assessgap -f -n 50 -o cat' ;;
access) ssh -t "$host" "sudo tail -n 20 -F /var/log/caddy/access.log | jq --unbuffered -r '[(.ts|localtime|strftime(\"%H:%M:%S\")), .status, .request.method, .request.uri, ((.duration*1000)|floor|tostring)+\"ms\"] | @tsv'" ;;
errors)
	ssh "$host" 'sudo journalctl -u assessgap --since -24h -o cat | grep -E "level=(WARN|ERROR)" | tail -n 50 || true
echo "--- 5xx responses (last 50):"
sudo jq -r "select(.status >= 500) | [(.ts|localtime|strftime(\"%m-%d %H:%M:%S\")), .status, .request.uri] | @tsv" /var/log/caddy/access.log | tail -n 50' ;;
*) echo "usage: ops/logs.sh [app|access|errors]" >&2; exit 2 ;;
esac
