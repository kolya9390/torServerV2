#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
script="$root/.github/scripts/extract-release-notes.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cat > "$tmp/CHANGELOG.md" <<'CHANGELOG'
# Changelog

## [Unreleased]

### Added

- Future change.

## [1.0.0-beta.1] - 2026-07-15

### Added

- Stable CLI JSON output.

### Fixed

- Bounded metadata wait.

## [0.9.0] - 2026-01-01

### Added

- Old change.
CHANGELOG

"$script" "$tmp/CHANGELOG.md" 1.0.0-beta.1 "$tmp/release-notes.md"
grep -Fq '## TorrServerV2 v1.0.0-beta.1' "$tmp/release-notes.md"
grep -Fq -- '- Stable CLI JSON output.' "$tmp/release-notes.md"
grep -Fq -- '- Bounded metadata wait.' "$tmp/release-notes.md"

if grep -Fq 'Future change.' "$tmp/release-notes.md" || grep -Fq 'Old change.' "$tmp/release-notes.md"; then
  echo "release notes crossed a changelog section boundary" >&2
  exit 1
fi

cp "$tmp/CHANGELOG.md" "$tmp/duplicate.md"
cat >> "$tmp/duplicate.md" <<'CHANGELOG'

## [1.0.0-beta.1] - 2026-07-16

### Fixed

- Duplicate release.
CHANGELOG

if "$script" "$tmp/duplicate.md" 1.0.0-beta.1 "$tmp/duplicate.out" >/dev/null 2>&1; then
  echo "extractor accepted duplicate version sections" >&2
  exit 1
fi

if "$script" "$tmp/CHANGELOG.md" 1.0.0-beta.2 "$tmp/missing.out" >/dev/null 2>&1; then
  echo "extractor accepted a missing version section" >&2
  exit 1
fi

sed 's/2026-07-15/15-07-2026/' "$tmp/CHANGELOG.md" > "$tmp/bad-date.md"
if "$script" "$tmp/bad-date.md" 1.0.0-beta.1 "$tmp/bad-date.out" >/dev/null 2>&1; then
  echo "extractor accepted a non-ISO release date" >&2
  exit 1
fi

if "$script" "$tmp/CHANGELOG.md" latest "$tmp/invalid.out" >/dev/null 2>&1; then
  echo "extractor accepted a non-SemVer release" >&2
  exit 1
fi

unreleased="$(awk '/^## \[Unreleased\]$/ { capture = 1; next } capture && /^## \[/ { exit } capture { print }' "$root/CHANGELOG.md")"
for category in Added Changed Fixed Security Deprecated Removed; do
  if ! grep -Fxq "### $category" <<< "$unreleased"; then
    echo "Unreleased is missing the $category category" >&2
    exit 1
  fi
done

for document in CHANGELOG.md VERSIONING.md; do
  if ! grep -Fq "($document)" "$root/README.md"; then
    echo "README does not link to $document" >&2
    exit 1
  fi

  [[ -f "$root/$document" ]]
done

echo "release notes policy tests passed"
