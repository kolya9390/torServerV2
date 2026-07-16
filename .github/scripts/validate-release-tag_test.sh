#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
validator="$script_dir/validate-release-tag.sh"

assert_valid() {
  local tag="$1"
  local channel="$2"
  local prerelease="$3"
  local output

  output="$("$validator" "$tag")"
  grep -Fxq "tag=$tag" <<<"$output"
  grep -Fxq "version=${tag#v}" <<<"$output"
  grep -Fxq "channel=$channel" <<<"$output"
  grep -Fxq "prerelease=$prerelease" <<<"$output"
}

assert_invalid() {
  local tag="$1"

  if "$validator" "$tag" >/dev/null 2>&1; then
    echo "expected invalid tag to fail: $tag" >&2
    exit 1
  fi
}

assert_valid "v0.1.0" "stable" "false"
assert_valid "v1.0.0" "stable" "false"
assert_valid "v12.34.56-alpha.1" "alpha" "true"
assert_valid "v1.0.0-beta.12" "beta" "true"
assert_valid "v2.3.4-rc.2" "rc" "true"

for tag in \
  "1.0.0" \
  "v1" \
  "v1.0" \
  "v01.0.0" \
  "v1.00.0" \
  "v1.0.00" \
  "v1.0.0-alpha" \
  "v1.0.0-alpha.0" \
  "v1.0.0-beta.01" \
  "v1.0.0-RC.1" \
  "v1.0.0-preview.1" \
  "v1.0.0+build.1" \
  "v1.0.0-beta.1-extra"; do
  assert_invalid "$tag"
done

echo "release tag validation tests passed"
