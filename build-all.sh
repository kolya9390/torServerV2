#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
metadata="$($root/.github/scripts/build-metadata.sh ldflags "$root")"
mkdir -p "$root/dist"

targets=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
  "windows amd64"
)

for target in "${targets[@]}"; do
  read -r goos goarch <<<"$target"
  extension=""
  if [[ "$goos" == "windows" ]]; then
    extension=".exe"
  fi

  output="$root/dist/torrserver-$goos-$goarch$extension"
  echo "Building $goos/$goarch -> $output"
  (
    cd "$root/server"
    GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
      go build -ldflags "-w -s $metadata" -o "$output" ./cmd
  )
done
