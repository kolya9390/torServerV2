#!/usr/bin/env bash
set -euo pipefail

mode="${1:-env}"
repo="${2:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"

git_available=false
if git -C "$repo" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  git_available=true
fi

dirty="${DIRTY:-}"
if [[ -z "$dirty" ]]; then
  if [[ "$git_available" == true ]]; then
    if [[ -n "$(git -C "$repo" status --porcelain --untracked-files=normal)" ]]; then
      dirty="modified"
    else
      dirty="clean"
    fi
  else
    dirty="unknown"
  fi
fi

version="${VERSION:-}"
if [[ -z "$version" ]]; then
  if [[ "$git_available" == true ]]; then
    version="$(git -C "$repo" describe --tags --always 2>/dev/null || git -C "$repo" rev-parse --short=12 HEAD)"
    if [[ "$version" != v* && "$version" != *-g* ]]; then
      version="dev-$version"
    fi
    if [[ "$dirty" == "modified" && "$version" != *-dirty ]]; then
      version="$version-dirty"
    fi
  else
    version="dev"
  fi
fi

commit="${COMMIT:-}"
if [[ -z "$commit" ]]; then
  if [[ "$git_available" == true ]]; then
    commit="$(git -C "$repo" rev-parse HEAD)"
  else
    commit="unknown"
  fi
fi

build_time="${BUILD_TIME:-unknown}"

for value in "$version" "$commit" "$build_time" "$dirty"; do
  if [[ "$value" =~ [[:space:]] ]]; then
    echo "build metadata values must not contain whitespace" >&2
    exit 2
  fi
done

case "$mode" in
  env)
    printf 'version=%s\ncommit=%s\nbuild_time=%s\ndirty=%s\n' \
      "$version" "$commit" "$build_time" "$dirty"
    ;;
  ldflags)
    printf '%s' \
      "-X=server/version.version=$version " \
      "-X=server/version.commit=$commit " \
      "-X=server/version.buildTime=$build_time " \
      "-X=server/version.dirtyState=$dirty"
    ;;
  *)
    echo "usage: $0 [env|ldflags] [repository]" >&2
    exit 2
    ;;
esac
