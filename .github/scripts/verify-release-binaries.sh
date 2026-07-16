#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: $0 <release-dir> <version-without-v> <commit>" >&2
  exit 2
fi

release_dir="$1"
version="$2"
commit="$3"
expected=(
  "torrserver-$version-linux-amd64"
  "torrserver-$version-linux-arm64"
  "torrserver-$version-darwin-amd64"
  "torrserver-$version-darwin-arm64"
  "torrserver-$version-windows-amd64.exe"
)

for name in "${expected[@]}"; do
  binary="$release_dir/$name"
  [[ -f "$binary" ]] || { echo "missing release binary: $name" >&2; exit 1; }

  build_info="$(go version -m "$binary")"
  grep -Fq -- "-X=server/version.version=v$version" <<< "$build_info" || {
    echo "binary $name has wrong embedded version" >&2
    exit 1
  }
  grep -Fq -- "-X=server/version.commit=$commit" <<< "$build_info" || {
    echo "binary $name has wrong embedded commit" >&2
    exit 1
  }
  grep -Fq -- "-X=server/version.dirtyState=clean" <<< "$build_info" || {
    echo "binary $name is not marked clean" >&2
    exit 1
  }
done

case "$(uname -s)/$(uname -m)" in
  Linux/x86_64) host="torrserver-$version-linux-amd64" ;;
  Linux/aarch64|Linux/arm64) host="torrserver-$version-linux-arm64" ;;
  Darwin/x86_64) host="torrserver-$version-darwin-amd64" ;;
  Darwin/arm64) host="torrserver-$version-darwin-arm64" ;;
  *) echo "unsupported verification host: $(uname -s)/$(uname -m)" >&2; exit 1 ;;
esac

chmod +x "$release_dir/$host"
version_json="$("$release_dir/$host" version --output json)"
node -e '
const value = JSON.parse(process.argv[1]);
const expectedVersion = `v${process.argv[2]}`;
const expectedCommit = process.argv[3];
if (!value.ok || value.data?.version !== expectedVersion || value.data?.commit !== expectedCommit || value.data?.dirty !== "clean") {
  throw new Error("host binary version JSON does not match release metadata");
}
' "$version_json" "$version" "$commit"

printf 'verified %d release binaries and host CLI metadata\n' "${#expected[@]}"
