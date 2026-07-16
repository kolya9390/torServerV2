#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
script="$root/.github/scripts/prepare-release-assets.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

version="1.0.0-beta.1"
names=(
  "torrserver-$version-linux-amd64"
  "torrserver-$version-linux-arm64"
  "torrserver-$version-darwin-amd64"
  "torrserver-$version-darwin-arm64"
  "torrserver-$version-windows-amd64.exe"
)

create_matrix() {
  local directory="$1"
  mkdir -p "$directory"

  for name in "${names[@]}"; do
    mkdir -p "$directory/$name"
    printf '%s\n' "$name" > "$directory/$name/$name"
  done
}

artifacts="$tmp/artifacts"
release="$tmp/release"
create_matrix "$artifacts"
"$script" "$artifacts" "$release" "$version"

for name in "${names[@]}"; do
  [[ -f "$release/$name" ]]
done

manifest="$release/torrserver-$version-SHA256SUMS"
[[ -f "$manifest" ]]
[[ "$(wc -l < "$manifest" | tr -d ' ')" == "5" ]]

missing="$tmp/missing"
create_matrix "$missing"
rm "$missing/${names[0]}/${names[0]}"
if "$script" "$missing" "$tmp/missing-release" "$version" >/dev/null 2>&1; then
  echo "collector accepted an incomplete matrix" >&2
  exit 1
fi

extra="$tmp/extra"
create_matrix "$extra"
printf 'unexpected\n' > "$extra/unexpected.txt"
if "$script" "$extra" "$tmp/extra-release" "$version" >/dev/null 2>&1; then
  echo "collector accepted an unexpected artifact" >&2
  exit 1
fi

duplicate="$tmp/duplicate"
create_matrix "$duplicate"
mkdir -p "$duplicate/copy"
cp "$duplicate/${names[1]}/${names[1]}" "$duplicate/copy/${names[1]}"
rm "$duplicate/${names[2]}/${names[2]}"
if "$script" "$duplicate" "$tmp/duplicate-release" "$version" >/dev/null 2>&1; then
  echo "collector accepted a duplicate that masked a missing target" >&2
  exit 1
fi

stale="$tmp/stale"
create_matrix "$stale"
mkdir -p "$tmp/stale-release"
printf 'stale\n' > "$tmp/stale-release/unexpected.txt"
if "$script" "$stale" "$tmp/stale-release" "$version" >/dev/null 2>&1; then
  echo "collector accepted a non-empty release directory" >&2
  exit 1
fi

echo "release asset collection tests passed"
