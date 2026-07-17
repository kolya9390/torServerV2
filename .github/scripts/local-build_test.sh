#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

dist="$tmp/dist"
metadata_env=(
  "VERSION=v1.0.0-beta.test"
  "COMMIT=0123456789abcdef0123456789abcdef01234567"
  "BUILD_TIME=2026-07-17T00:00:00Z"
  "DIRTY=clean"
)

build_target() {
  local target="$1"

  env "${metadata_env[@]}" GOCACHE="${GOCACHE:-/private/tmp/torserverv2-gocache}" \
    make --no-print-directory -C "$root" "$target" BINARY_DIR="$dist" >/dev/null
}

assert_artifacts() {
  local expected actual
  expected=$'torrctl\ntorrserver'
  actual="$(find "$dist" -maxdepth 1 -type f -exec basename {} \; | sort)"
  if [[ "$actual" != "$expected" ]]; then
    printf 'native artifact set mismatch:\n%s\n' "$actual" >&2
    exit 1
  fi

  [[ -x "$dist/torrserver" ]] || { echo "torrserver is not executable" >&2; exit 1; }
  [[ -x "$dist/torrctl" ]] || { echo "torrctl is not executable" >&2; exit 1; }
  [[ ! -e "$dist/tsctl" ]] || { echo "legacy tsctl artifact remains" >&2; exit 1; }
}

assert_metadata() {
  local server_version cli_version server_metadata cli_metadata
  server_version="$($dist/torrserver --version)"
  cli_version="$($dist/torrctl --version)"
  server_metadata="${server_version#* }"
  cli_metadata="${cli_version#* }"

  [[ "$server_version" == torrserver\ v1.0.0-beta.test* ]] || {
    echo "unexpected torrserver version: $server_version" >&2
    exit 1
  }
  [[ "$cli_version" == torrctl\ v1.0.0-beta.test* ]] || {
    echo "unexpected torrctl version: $cli_version" >&2
    exit 1
  }
  [[ "$server_metadata" == "$cli_metadata" ]] || {
    printf 'metadata differs:\nserver=%s\ncli=%s\n' "$server_metadata" "$cli_metadata" >&2
    exit 1
  }
}

assert_dependency_boundaries() {
  local cli_deps server_deps forbidden
  cli_deps="$(cd "$root/server" && go list -deps -f '{{.ImportPath}}' ./cmd/torrctl)"
  server_deps="$(cd "$root/server" && go list -deps -f '{{.ImportPath}}' ./cmd/torrserver)"

  for forbidden in server/internal/daemon server/bootstrap server/torr github.com/anacrolix/torrent; do
    if grep -Eq "^${forbidden}(/|$)" <<<"$cli_deps"; then
      echo "torrctl links forbidden dependency: $forbidden" >&2
      exit 1
    fi
  done

  for forbidden in server/internal/cliapp server/internal/apiclient github.com/spf13/cobra golang.org/x/term; do
    if grep -Eq "^${forbidden}(/|$)" <<<"$server_deps"; then
      echo "torrserver links forbidden dependency: $forbidden" >&2
      exit 1
    fi
  done
}

mkdir -p "$dist"
build_target build-server
[[ -x "$dist/torrserver" && ! -e "$dist/torrctl" ]] || {
  echo "build-server did not produce an isolated daemon artifact" >&2
  exit 1
}

printf 'legacy\n' > "$dist/tsctl"
build_target build-cli
assert_artifacts
assert_metadata
assert_dependency_boundaries

env "${metadata_env[@]}" make --no-print-directory -C "$root" clean-binaries BINARY_DIR="$dist" >/dev/null
[[ ! -e "$dist" ]] || { echo "clean-binaries left the output directory" >&2; exit 1; }

build_target build
assert_artifacts
assert_metadata

echo "local split-build tests passed"
