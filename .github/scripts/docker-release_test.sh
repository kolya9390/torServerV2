#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
dockerfile="$root/Dockerfile"
workflow="$root/.github/workflows/docker.yml"

grep -Fq -- '-o /torrserver ./cmd/torrserver' "$dockerfile"
grep -Fq -- 'ENTRYPOINT ["/sbin/tini", "--", "/usr/bin/torrserver", "serve"]' "$dockerfile"
if grep -Fq 'torrctl' "$dockerfile"; then
  echo "Docker image includes or builds torrctl" >&2
  exit 1
fi

grep -Fq 'platforms: linux/amd64,linux/arm64' "$workflow"
grep -Fq 'type=raw,value=latest,enable=${{ inputs.prerelease == false }}' "$workflow"
grep -Fq 'type=semver,pattern={{major}}.{{minor}},value=${{ inputs.release_tag }},enable=${{ inputs.prerelease == false }}' "$workflow"

echo "daemon-only Docker release tests passed"
