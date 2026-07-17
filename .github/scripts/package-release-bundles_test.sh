#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
script="$root/.github/scripts/package-release-bundles.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

version="1.0.0-beta.7"
repository="$tmp/repository"
release="$tmp/release"
mkdir -p "$repository/server/config" "$repository/server/internal" "$repository/server/cmd/internal"
cp -R "$root/server/internal/releasebundle" "$repository/server/internal/"
cp -R "$root/server/cmd/internal/releasebundle" "$repository/server/cmd/internal/"
cp "$root/server/go.mod" "$root/server/go.sum" "$repository/server/"
printf 'debug:\n  enabled: false\n' > "$repository/server/config/config.yml"
mkdir -p "$release"

for target in darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64; do
  extension=""
  [[ "$target" == windows-* ]] && extension=".exe"

  for product in torrserver torrctl; do
    printf '%s-%s\n' "$product" "$target" > "$release/$product-$version-$target$extension"
  done
done

"$script" "$repository" "$release" "$version"

expected_bundles=$'TorrServerV2-v1.0.0-beta.7-darwin-amd64.tar.gz\nTorrServerV2-v1.0.0-beta.7-darwin-arm64.tar.gz\nTorrServerV2-v1.0.0-beta.7-linux-amd64.tar.gz\nTorrServerV2-v1.0.0-beta.7-linux-arm64.tar.gz\nTorrServerV2-v1.0.0-beta.7-windows-amd64.zip'
actual_bundles="$(find "$release" -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.zip' \) -exec basename {} \; | sort)"
[[ "$actual_bundles" == "$expected_bundles" ]] || {
  echo "unexpected platform bundles:" >&2
  printf '%s\n' "$actual_bundles" >&2
  exit 1
}

manifest="$release/torrserver-$version-SHA256SUMS"
[[ "$(wc -l < "$manifest" | tr -d ' ')" == "15" ]]

tar -tzf "$release/TorrServerV2-v$version-linux-amd64.tar.gz" > "$tmp/tar-members"
expected_tar=$'TorrServerV2-v1.0.0-beta.7-linux-amd64/\nTorrServerV2-v1.0.0-beta.7-linux-amd64/config.example.yml\nTorrServerV2-v1.0.0-beta.7-linux-amd64/torrctl\nTorrServerV2-v1.0.0-beta.7-linux-amd64/torrserver'
[[ "$(cat "$tmp/tar-members")" == "$expected_tar" ]]

expected_zip=$'TorrServerV2-v1.0.0-beta.7-windows-amd64/\nTorrServerV2-v1.0.0-beta.7-windows-amd64/config.example.yml\nTorrServerV2-v1.0.0-beta.7-windows-amd64/torrctl.exe\nTorrServerV2-v1.0.0-beta.7-windows-amd64/torrserver.exe'
actual_zip="$(unzip -Z1 "$release/TorrServerV2-v$version-windows-amd64.zip")"
[[ "$actual_zip" == "$expected_zip" ]] || {
  echo "unexpected zip members:" >&2
  printf '%s\n' "$actual_zip" >&2
  exit 1
}

if "$script" "$repository" "$release" "$version" >/dev/null 2>&1; then
  echo "packager accepted stale existing bundles" >&2
  exit 1
fi

echo "release bundle packaging tests passed"
