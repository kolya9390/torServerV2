#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: $0 <changelog> <semver-without-v> <output>" >&2
  exit 2
fi

changelog="$1"
version="$2"
output="$3"

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)\.[0-9]+)?$ ]]; then
  echo "invalid release version: $version" >&2
  exit 2
fi

if [[ ! -f "$changelog" ]]; then
  echo "changelog not found: $changelog" >&2
  exit 1
fi

heading_prefix="## [$version] - "
heading_count="$(awk -v prefix="$heading_prefix" 'index($0, prefix) == 1 { count++ } END { print count + 0 }' "$changelog")"
if [[ "$heading_count" -ne 1 ]]; then
  echo "expected exactly one changelog section for $version, found $heading_count" >&2
  exit 1
fi

heading="$(awk -v prefix="$heading_prefix" 'index($0, prefix) == 1 { print; exit }' "$changelog")"
release_date="${heading#"$heading_prefix"}"
if [[ ! "$release_date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
  echo "release section must use an ISO date: $heading" >&2
  exit 1
fi

tmp="${output}.tmp.$$"
trap 'rm -f "$tmp"' EXIT

{
  printf '## TorrServerV2 v%s\n\n' "$version"
  awk -v prefix="$heading_prefix" '
    index($0, prefix) == 1 { capture = 1; next }
    capture && /^## \[/ { exit }
    capture { print }
  ' "$changelog"
} > "$tmp"

if ! grep -q '^### ' "$tmp" || ! grep -q '^- ' "$tmp"; then
  echo "release section $version must contain a category and at least one entry" >&2
  exit 1
fi

mv "$tmp" "$output"
trap - EXIT
