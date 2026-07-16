#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
script="$root/.github/scripts/build-metadata.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

assert_line() {
  local output="$1"
  local expected="$2"
  if ! grep -Fxq "$expected" <<<"$output"; then
    printf 'missing %q in:\n%s\n' "$expected" "$output" >&2
    exit 1
  fi
}

no_git="$tmp/no-git"
mkdir -p "$no_git"
output="$($script env "$no_git")"
assert_line "$output" "version=dev"
assert_line "$output" "commit=unknown"
assert_line "$output" "build_time=unknown"
assert_line "$output" "dirty=unknown"

repo="$tmp/repo"
git init -q "$repo"
git -C "$repo" config user.name "Build Metadata Test"
git -C "$repo" config user.email "build-metadata@example.test"
printf 'one\n' > "$repo/file.txt"
git -C "$repo" add file.txt
git -C "$repo" commit -qm "initial"

commit="$(git -C "$repo" rev-parse HEAD)"
short="$(git -C "$repo" rev-parse --short HEAD)"
output="$($script env "$repo")"
assert_line "$output" "version=dev-$short"
assert_line "$output" "commit=$commit"
assert_line "$output" "dirty=clean"

git -C "$repo" tag -a v1.0.0-beta.1 -m "test tag"
output="$($script env "$repo")"
assert_line "$output" "version=v1.0.0-beta.1"

printf 'dirty\n' > "$repo/untracked.txt"
output="$($script env "$repo")"
assert_line "$output" "version=v1.0.0-beta.1-dirty"
assert_line "$output" "dirty=modified"

output="$(VERSION=v9.8.7 COMMIT=override BUILD_TIME=2026-07-15T08:00:00Z DIRTY=clean "$script" env "$no_git")"
assert_line "$output" "version=v9.8.7"
assert_line "$output" "commit=override"
assert_line "$output" "build_time=2026-07-15T08:00:00Z"
assert_line "$output" "dirty=clean"

ldflags="$(VERSION=v9.8.7 COMMIT=override BUILD_TIME=unknown DIRTY=clean "$script" ldflags "$no_git")"
[[ "$ldflags" == *"-X=server/version.version=v9.8.7"* ]]
[[ "$ldflags" == *"-X=server/version.commit=override"* ]]

echo "build metadata policy tests passed"
