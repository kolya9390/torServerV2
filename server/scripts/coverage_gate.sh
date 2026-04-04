#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SERVER_DIR="${ROOT_DIR}/server"

CORE_THRESHOLD="${CORE_THRESHOLD:-4.0}"
API_THRESHOLD="${API_THRESHOLD:-12.0}"

run_cov() {
  local pkg="$1"
  local out
  out="$(cd "${SERVER_DIR}" && GOCACHE=/tmp/go-build-cache go test "${pkg}" -cover)"
  echo "${out}" >&2
  local cov
  cov="$(echo "${out}" | sed -nE 's/.*coverage: ([0-9.]+)% of statements.*/\1/p' | tail -n 1)"
  if [[ -z "${cov}" ]]; then
    echo "coverage parse failed for ${pkg}" >&2
    return 1
  fi
  echo "${cov}"
}

core_cov="$(run_cov ./torr)"
api_cov="$(run_cov ./web/api)"

echo "core coverage (./torr): ${core_cov}% (threshold ${CORE_THRESHOLD}%)"
echo "api coverage (./web/api): ${api_cov}% (threshold ${API_THRESHOLD}%)"

awk -v c="${core_cov}" -v t="${CORE_THRESHOLD}" 'BEGIN{exit (c+0>=t+0)?0:1}' || {
  echo "coverage gate failed: core coverage ${core_cov}% < ${CORE_THRESHOLD}%"
  exit 1
}

awk -v c="${api_cov}" -v t="${API_THRESHOLD}" 'BEGIN{exit (c+0>=t+0)?0:1}' || {
  echo "coverage gate failed: api coverage ${api_cov}% < ${API_THRESHOLD}%"
  exit 1
}

echo "coverage gate passed"
