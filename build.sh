#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

mkdir -p bin
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-X main.version=${VERSION}" \
  -o bin/muster .

echo "Built: bin/muster ${VERSION}"
