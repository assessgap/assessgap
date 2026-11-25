#!/bin/bash
# Build the assessgap binary for the droplet and install it; systemd runs
# "assessgap serve".  ops/deploy/deploy.sh
set -euo pipefail
cd "$(dirname "$0")/../.."
host=${DEPLOY_HOST:-assessgap-web}
out=$(mktemp -d)/assessgap
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "$out" ./cmd/assessgap
scp -q "$out" ops/deploy/assessgap.service ops/deploy/Caddyfile "$host:/tmp/"
ssh "$host" 'set -euo pipefail
  sudo install -m 755 -o root -g root /tmp/assessgap /srv/assessgap/assessgap
  sudo install -m 644 /tmp/assessgap.service /etc/systemd/system/assessgap.service
  sudo install -m 644 /tmp/Caddyfile /etc/caddy/Caddyfile
  sudo install -d -o caddy -g caddy -m 755 /var/log/caddy
  sudo systemctl daemon-reload
  sudo systemctl enable -q assessgap
  sudo systemctl restart assessgap
  sudo -u caddy caddy validate --config /etc/caddy/Caddyfile >/dev/null
  sudo systemctl reload caddy
  sleep 1; curl -fsS -o /dev/null -w "healthz %{http_code}\n" http://127.0.0.1:8080/healthz'
