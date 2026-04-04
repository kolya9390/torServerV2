#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if ! command -v golangci-lint >/dev/null 2>&1; then
  echo "golangci-lint not found in PATH" >&2
  echo "Install: https://golangci-lint.run/welcome/install/" >&2
  exit 1
fi

export GOCACHE="${GOCACHE:-/tmp/go-build-cache}"
export GOLANGCI_LINT_CACHE="${GOLANGCI_LINT_CACHE:-/tmp/golangci-lint-cache}"
LINT_PROFILE="${1:-base}"
LINT_CONFIG=".golangci.yml"

if [[ "$LINT_PROFILE" == "strict" ]]; then
  LINT_CONFIG=".golangci-strict.yml"
fi

mkdir -p "$GOCACHE" "$GOLANGCI_LINT_CACHE"

echo "[lint] running golangci-lint profile=$LINT_PROFILE config=$LINT_CONFIG"
golangci-lint run --config "$LINT_CONFIG" ./...
echo "[lint] passed"
