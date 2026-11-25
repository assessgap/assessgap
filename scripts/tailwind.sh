#!/bin/bash
# Download the Tailwind CSS v4 standalone CLI into bin/ (needed only to change styles;
# the generated internal/web/static/app.css is committed).
set -euo pipefail
cd "$(dirname "$0")/.."
version=${TAILWIND_VERSION:-v4.3.5}
os=$(uname -s | tr '[:upper:]' '[:lower:]'); [ "$os" = darwin ] && os=macos
arch=$(uname -m); case $arch in x86_64) arch=x64 ;; aarch64|arm64) arch=arm64 ;; esac
mkdir -p bin
curl -fsSL -o bin/tailwindcss "https://github.com/tailwindlabs/tailwindcss/releases/download/$version/tailwindcss-$os-$arch"
chmod +x bin/tailwindcss
bin/tailwindcss --help | head -1
