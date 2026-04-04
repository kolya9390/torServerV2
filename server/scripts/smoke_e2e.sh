#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SERVER_DIR="${ROOT_DIR}/server"
BIN_PATH="/tmp/torrserver-smoke"

echo "[smoke] build binary"
(
  cd "${SERVER_DIR}"
  GOCACHE=/tmp/go-build-cache go build -o "${BIN_PATH}" ./cmd
)

echo "[smoke] verify CLI entrypoints"
"${BIN_PATH}" --help >/dev/null

echo "[smoke] run focused smoke tests"
(
  cd "${SERVER_DIR}"
  GOCACHE=/tmp/go-build-cache go test ./web -run 'TestRequestIDMiddleware|TestMetricsEndpoint|TestSecurityHeadersMiddleware'
  GOCACHE=/tmp/go-build-cache go test ./web/auth -run 'TestProtectedRouteUnauthorizedBlocked|TestProtectedRouteAuthorizedPasses'
)

echo "[smoke] passed"
