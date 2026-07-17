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
grep -Fq './.github/scripts/package-release-bundles.sh . release' "$release"
grep -Fq './.github/scripts/verify-release-bundles.sh . release' "$release"

validate_setup_line="$(grep -n -m1 'uses: actions/setup-go@v6' "$release" | cut -d: -f1)"
release_test_line="$(grep -n -m1 'run: make test-release' "$release" | cut -d: -f1)"
[[ -n "$validate_setup_line" && -n "$release_test_line" && "$validate_setup_line" -lt "$release_test_line" ]] || {
  echo "release helper tests run before the pinned Go toolchain is installed" >&2
  exit 1
}

grep -Fq 'needs: [test, lint]' "$ci"
grep -Fq 'run: make e2e' "$ci"
grep -Fq 'dist/torrserver' "$ci"
grep -Fq 'dist/torrctl' "$ci"

for workflow in "$ci" "$release"; do
  grep -Fq 'uses: actions/checkout@v6' "$workflow"
  grep -Fq 'uses: actions/setup-go@v6' "$workflow"
  grep -Fq 'uses: golangci/golangci-lint-action@v9' "$workflow"
  grep -Fq 'version: v2.10.1' "$workflow"

  if grep -Fq 'install-mode: goinstall' "$workflow"; then
    echo "golangci-lint goinstall mode is unsupported by the workflow contract: $workflow" >&2
    exit 1
  fi
done

grep -Fq 'uses: actions/upload-artifact@v7' "$ci"
grep -Fq 'uses: actions/upload-artifact@v7' "$release"
grep -Fq 'uses: actions/download-artifact@v8' "$release"

echo "CI and release workflow contract tests passed"
