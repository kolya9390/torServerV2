#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 4 ]]; then
  echo "usage: $0 <repository-root> <release-dir> <semver-without-v> <commit>" >&2
  exit 2
fi

repository_root="$(cd "$1" && pwd)"
release_dir="$(cd "$2" && pwd)"
version="$3"
commit="$4"

(
  cd "$repository_root/server"
  go run ./cmd/internal/releasebundle verify "$release_dir" "$version" "$commit"
)

printf 'verified 5 platform bundles and aggregate checksums for %s\n' "$version"
