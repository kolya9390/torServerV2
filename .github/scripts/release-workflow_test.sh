#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ci="$root/.github/workflows/ci.yml"
release="$root/.github/workflows/release.yml"

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
  goos="${target%/*}"
  goarch="${target#*/}"
  grep -Fq -- "- goos: $goos" "$release"
  grep -Fq -- "goarch: $goarch" "$release"
done

grep -Fq 'needs: [validate, test, lint]' "$release"
grep -Fq 'run: make e2e' "$release"
grep -Fq 'dist/torrserver-${{ needs.validate.outputs.version }}-${{ matrix.goos }}-${{ matrix.goarch }}*' "$release"
grep -Fq 'dist/torrctl-${{ needs.validate.outputs.version }}-${{ matrix.goos }}-${{ matrix.goarch }}*' "$release"
grep -Fq 'if-no-files-found: error' "$release"
grep -Fq 'pattern: release-binaries-*' "$release"

grep -Fq 'needs: [test, lint]' "$ci"
grep -Fq 'run: make e2e' "$ci"
grep -Fq 'dist/torrserver' "$ci"
grep -Fq 'dist/torrctl' "$ci"

echo "CI and release workflow contract tests passed"
