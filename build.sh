#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

if command -v module >/dev/null 2>&1; then
  source /etc/profile.d/lmod.sh 2>/dev/null || true
fi
module load go/24.2 2>/dev/null || true

mkdir -p bin
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-X main.version=${VERSION}" \
  -o bin/muster .

echo "Built: bin/muster ${VERSION}"
