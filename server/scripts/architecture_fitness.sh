#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-/tmp/go-build-cache}"
mkdir -p "$GOCACHE"

echo "[fitness] running architecture fitness gates"
go test ./internal/archtest -count=1
echo "[fitness] passed"
