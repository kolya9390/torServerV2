#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
dist_dir="${DIST_DIR:-$root/dist}"
go_cmd="${GO_CMD:-go}"

supported_targets=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
)

if [[ $# -gt 0 ]]; then
  targets=("$@")
else
  targets=("${supported_targets[@]}")
fi

is_supported_target() {
  local candidate="$1"
  local supported

  for supported in "${supported_targets[@]}"; do
    if [[ "$candidate" == "$supported" ]]; then
      return 0
    fi
  done

  return 1
}

artifact_path() {
  local binary="$1"
  local target="$2"
  local goos="${target%/*}"
  local goarch="${target#*/}"
  local extension=""
  if [[ "$goos" == "windows" ]]; then
    extension=".exe"
  fi

  printf '%s/%s-%s-%s%s' "$dist_dir" "$binary" "$goos" "$goarch" "$extension"
}

for target in "${targets[@]}"; do
  if ! is_supported_target "$target"; then
    echo "unsupported build target: $target" >&2
    exit 2
  fi
done

metadata="$($root/.github/scripts/build-metadata.sh ldflags "$root")"
mkdir -p "$dist_dir"
dist_dir="$(cd "$dist_dir" && pwd)"

# Remove all canonical outputs first so a failed build cannot leave an older
# mixed executable looking like a current split artifact.
rm -f "$dist_dir/torrserver" "$dist_dir/torrctl" "$dist_dir/tsctl"
for target in "${supported_targets[@]}"; do
  rm -f "$(artifact_path torrserver "$target")" "$(artifact_path torrctl "$target")"
done

for target in "${targets[@]}"; do
  goos="${target%/*}"
  goarch="${target#*/}"

  for binary in torrserver torrctl; do
    output="$(artifact_path "$binary" "$target")"
    package="./cmd/$binary"
    echo "Building $binary for $goos/$goarch -> $output"
    (
      cd "$root/server"
      GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
        "$go_cmd" build -ldflags "-w -s $metadata" -o "$output" "$package"
    )

    if [[ ! -s "$output" ]]; then
      echo "build did not create artifact: $output" >&2
      exit 1
    fi

    if [[ "$goos" != "windows" ]]; then
      chmod +x "$output"
    fi
  done
done
