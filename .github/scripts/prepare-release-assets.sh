#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: $0 <artifacts-dir> <release-dir> <semver-without-v>" >&2
  exit 2
fi

artifacts_dir="$1"
release_dir="$2"
version="$3"

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)\.[0-9]+)?$ ]]; then
  echo "invalid release version: $version" >&2
  exit 2
fi

expected=(
  "torrserver-$version-linux-amd64"
  "torrserver-$version-linux-arm64"
  "torrserver-$version-darwin-amd64"
  "torrserver-$version-darwin-arm64"
  "torrserver-$version-windows-amd64.exe"
  "torrctl-$version-linux-amd64"
  "torrctl-$version-linux-arm64"
  "torrctl-$version-darwin-amd64"
  "torrctl-$version-darwin-arm64"
  "torrctl-$version-windows-amd64.exe"
)

discovered_count="$(find "$artifacts_dir" -type f -print | wc -l | tr -d ' ')"
if [[ "$discovered_count" -ne ${#expected[@]} ]]; then
  echo "release matrix contains $discovered_count files; expected ${#expected[@]}" >&2
  find "$artifacts_dir" -type f -print | sort >&2
  exit 1
fi

if [[ -e "$release_dir" ]] && [[ -n "$(find "$release_dir" -mindepth 1 -print -quit)" ]]; then
  echo "release directory must be empty: $release_dir" >&2
  exit 1
fi

mkdir -p "$release_dir"

for name in "${expected[@]}"; do
  match_count="$(find "$artifacts_dir" -type f -name "$name" -print | wc -l | tr -d ' ')"
  if [[ "$match_count" -ne 1 ]]; then
    echo "expected exactly one $name artifact, found $match_count" >&2
    exit 1
  fi

  source="$(find "$artifacts_dir" -type f -name "$name" -print -quit)"
  cp "$source" "$release_dir/$name"
done

manifest="torrserver-$version-SHA256SUMS"
if command -v sha256sum >/dev/null 2>&1; then
  (
    cd "$release_dir"
    sha256sum "${expected[@]}" > "$manifest"
    sha256sum --check "$manifest"
  )
elif command -v shasum >/dev/null 2>&1; then
  (
    cd "$release_dir"
    shasum -a 256 "${expected[@]}" > "$manifest"
    shasum -a 256 --check "$manifest"
  )
else
  echo "sha256sum or shasum is required" >&2
  exit 1
fi

printf 'prepared %d binaries and %s\n' "${#expected[@]}" "$manifest"
