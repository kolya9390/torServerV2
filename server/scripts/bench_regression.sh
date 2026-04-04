#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASELINE="${ROOT}/benchmarks/baseline.json"
LATEST="${ROOT}/benchmarks/results/latest.json"
REPORT="${ROOT}/benchmarks/results/report.md"
THRESHOLD="${PERF_REGRESSION_THRESHOLD:-0.15}"

if [[ ! -f "${LATEST}" ]]; then
  echo "[bench-regression] latest benchmark file not found: ${LATEST}"
  exit 1
fi

if [[ ! -f "${BASELINE}" ]]; then
  echo "[bench-regression] baseline missing, creating initial baseline from latest"
  cp "${LATEST}" "${BASELINE}"
fi

python3 "${ROOT}/scripts/bench_compare.py" \
  --baseline "${BASELINE}" \
  --latest "${LATEST}" \
  --threshold "${THRESHOLD}" \
  --report "${REPORT}"

echo "[bench-regression] report: ${REPORT}"
