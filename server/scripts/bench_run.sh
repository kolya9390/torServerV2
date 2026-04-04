#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT}/benchmarks/results"
mkdir -p "${OUT_DIR}"

RAW="${OUT_DIR}/latest.txt"
JSON="${OUT_DIR}/latest.json"

echo "[bench] running core benchmark suite"
GOCACHE="${GOCACHE:-/tmp/go-build-cache}" \
  go test ./torr/... \
  -run '^$' \
  -bench 'Benchmark(AdaptiveReadaheadProfile|StreamAdmissionProfile|TieredCacheReadHotProfile|TieredCacheReadWarmProfile)' \
  -benchmem \
  -count=3 \
  > "${RAW}"

python3 "${ROOT}/scripts/bench_parse.py" --input "${RAW}" --output "${JSON}"
echo "[bench] raw: ${RAW}"
echo "[bench] json: ${JSON}"
