#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: $0 <release-dir> <version-without-v> <commit>" >&2
  exit 2
fi

release_dir="$1"
version="$2"
commit="$3"
go_cmd="${GO_CMD:-go}"
expected=(
  "torrserver-$version-linux-amd64"
  "torrserver-$version-linux-arm64"
  "torrserver-$version-darwin-amd64"
  "torrserver-$version-darwin-arm64"
  "torrserver-$version-windows-amd64.exe"
  "torrctl-$version-linux-amd64"
  "torrctl-$version-linux-arm64"
  "torrctl-$version-darwin-amd64"
  "torrctl-$version-darwin-arm64"
  "torrctl-$version-windows-amd64.exe"
)

for name in "${expected[@]}"; do
  binary="$release_dir/$name"
  binary_name="${name%%-*}"
  expected_package="server/cmd/$binary_name"
  [[ -f "$binary" ]] || { echo "missing release binary: $name" >&2; exit 1; }

  build_info="$("$go_cmd" version -m "$binary")"
  awk -v expected="$expected_package" '$1 == "path" && $2 == expected { found = 1 } END { exit !found }' \
    <<< "$build_info" || {
    echo "binary $name was built from the wrong package; expected $expected_package" >&2
    exit 1
  }
  grep -Fq -- "-X=server/version.version=v$version" <<< "$build_info" || {
    echo "binary $name has wrong embedded version" >&2
    exit 1
  }
  grep -Fq -- "-X=server/version.commit=$commit" <<< "$build_info" || {
    echo "binary $name has wrong embedded commit" >&2
    exit 1
  }
  grep -Fq -- "-X=server/version.buildTime=unknown" <<< "$build_info" || {
    echo "binary $name has wrong embedded build time" >&2
    exit 1
  }
  grep -Fq -- "-X=server/version.dirtyState=clean" <<< "$build_info" || {
    echo "binary $name is not marked clean" >&2
    exit 1
  }
done

case "$(uname -s)/$(uname -m)" in
  Linux/x86_64) target="linux-amd64" ;;
  Linux/aarch64|Linux/arm64) target="linux-arm64" ;;
  Darwin/x86_64) target="darwin-amd64" ;;
  Darwin/arm64) target="darwin-arm64" ;;
  *) echo "unsupported verification host: $(uname -s)/$(uname -m)" >&2; exit 1 ;;
esac

host_server="$release_dir/torrserver-$version-$target"
host_cli="$release_dir/torrctl-$version-$target"
chmod +x "$host_server" "$host_cli"
server_version="$("$host_server" --version)"
version_json="$("$host_cli" --output=json version)"
node -e '
const value = JSON.parse(process.argv[1]);
const expectedVersion = `v${process.argv[2]}`;
const expectedCommit = process.argv[3];
const serverVersion = process.argv[4];
if (!value.ok || value.data?.version !== expectedVersion || value.data?.commit !== expectedCommit ||
    value.data?.build_time !== "unknown" || value.data?.dirty !== "clean") {
  throw new Error("host torrctl version JSON does not match release metadata");
}
if (!serverVersion.startsWith(`torrserver ${expectedVersion} `) || !serverVersion.includes(expectedCommit.slice(0, 12))) {
  throw new Error("host torrserver version output does not match release metadata");
}
' "$version_json" "$version" "$commit" "$server_version"

printf 'verified %d release binaries and matching host metadata\n' "${#expected[@]}"
