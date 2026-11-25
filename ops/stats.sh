#!/bin/bash
# Traffic summary for the last N days (default 7), computed on the web server
# from Caddy's access log. Counts only; no IPs or PINs leave the server.
#   ops/stats.sh [days]
set -euo pipefail
cd "$(dirname "$0")/.."
host=${DEPLOY_HOST:-assessgap-web}
ssh "$host" "sudo python3 - ${1:-7}" < ops/stats.py
