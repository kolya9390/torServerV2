#!/usr/bin/env bash

set -euo pipefail

readonly tag_pattern='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-(alpha|beta|rc)\.([1-9][0-9]*))?$'

if [[ $# -ne 1 ]]; then
  echo "usage: $0 vMAJOR.MINOR.PATCH[-alpha.N|-beta.N|-rc.N]" >&2
  exit 2
fi

tag="$1"
if [[ ! "$tag" =~ $tag_pattern ]]; then
  echo "invalid release tag: $tag" >&2
  exit 1
fi

channel="${BASH_REMATCH[5]:-stable}"
prerelease="true"
if [[ "$channel" == "stable" ]]; then
  prerelease="false"
fi

printf 'tag=%s\n' "$tag"
printf 'version=%s\n' "${tag#v}"
printf 'channel=%s\n' "$channel"
printf 'prerelease=%s\n' "$prerelease"
