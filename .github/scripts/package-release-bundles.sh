#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: $0 <repository-root> <release-dir> <semver-without-v>" >&2
  exit 2
fi

repository_root="$(cd "$1" && pwd)"
release_dir="$(cd "$2" && pwd)"
version="$3"

(
  cd "$repository_root/server"
  go run ./cmd/internal/releasebundle create "$repository_root" "$release_dir" "$version"
)

printf 'created 5 platform bundles and aggregate checksums for %s\n' "$version"
