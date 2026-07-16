#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 6 ]]; then
  echo "usage: $0 <repo> <output-dir> <version-without-v> <commit> <goos> <goarch>" >&2
  exit 2
fi

repo="$(cd "$1" && pwd)"
output_dir="$2"
version="$3"
commit="$4"
goos="$5"
goarch="$6"

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)\.[0-9]+)?$ ]]; then
  echo "invalid release version: $version" >&2
  exit 2
fi
if [[ ! "$commit" =~ ^[0-9a-f]{40}$ ]]; then
  echo "release commit must be a full lowercase git SHA" >&2
  exit 2
fi
if [[ "$(git -C "$repo" rev-parse HEAD)" != "$commit" ]]; then
  echo "release commit does not match repository HEAD" >&2
  exit 1
fi
if [[ -n "$(git -C "$repo" status --porcelain --untracked-files=normal)" ]]; then
  echo "release binary build requires a clean worktree" >&2
  exit 1
fi

target="$goos/$goarch"
case "$target" in
  linux/amd64|linux/arm64|darwin/amd64|darwin/arm64|windows/amd64) ;;
  *)
    echo "unsupported release target: $target" >&2
    exit 2
    ;;
esac

mkdir -p "$output_dir"
output_dir="$(cd "$output_dir" && pwd)"
extension=""
if [[ "$goos" == "windows" ]]; then
  extension=".exe"
fi

asset="torrserver-$version-$goos-$goarch$extension"
metadata="$(VERSION="v$version" COMMIT="$commit" BUILD_TIME=unknown DIRTY=clean \
  "$repo/.github/scripts/build-metadata.sh" ldflags "$repo")"

(
  cd "$repo/server"
  VERSION="v$version" COMMIT="$commit" BUILD_TIME=unknown DIRTY=clean \
    GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    go build -ldflags "-w -s $metadata" -o "$output_dir/$asset" ./cmd
)

printf '%s\n' "$output_dir/$asset"
