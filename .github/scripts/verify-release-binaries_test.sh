#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
script="$root/.github/scripts/verify-release-binaries.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

version="1.0.0-beta.4"
commit="0123456789abcdef0123456789abcdef01234567"
release="$tmp/release"
mkdir -p "$release"

names=(
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

for name in "${names[@]}"; do
  cat > "$release/$name" <<EOF
#!/usr/bin/env bash
set -euo pipefail
name="\$(basename "\$0")"
if [[ "\$name" == torrserver-* ]]; then
  printf 'torrserver v$version (test/test, commit ${commit:0:12})\n'
else
  printf '{"ok":true,"data":{"version":"v$version","commit":"$commit","build_time":"unknown","dirty":"clean"}}\n'
fi
EOF
  chmod +x "$release/$name"
done

fake_go="$tmp/fake-go"
cat > "$fake_go" <<'FAKE_GO'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == "version" && "$2" == "-m" ]]
name="$(basename "$3")"
binary_name="${name%%-*}"
package="server/cmd/$binary_name"
commit="$FIXTURE_COMMIT"
if [[ "${WRONG_BINARY:-}" == "$name" ]]; then
  commit="ffffffffffffffffffffffffffffffffffffffff"
fi
if [[ "${WRONG_PACKAGE_BINARY:-}" == "$name" ]]; then
  package="server/cmd/torrserver"
fi
printf '%s: go1.26.0\n\tpath\t%s\n\tbuild\t-ldflags="-w -s %s"\n' \
  "$3" \
  "$package" \
  "-X=server/version.version=v$FIXTURE_VERSION -X=server/version.commit=$commit -X=server/version.buildTime=unknown -X=server/version.dirtyState=clean"
FAKE_GO
chmod +x "$fake_go"

FIXTURE_VERSION="$version" FIXTURE_COMMIT="$commit" GO_CMD="$fake_go" \
  "$script" "$release" "$version" "$commit" >/dev/null

missing="$release/${names[5]}"
mv "$missing" "$missing.bak"
if FIXTURE_VERSION="$version" FIXTURE_COMMIT="$commit" GO_CMD="$fake_go" \
  "$script" "$release" "$version" "$commit" >/dev/null 2>&1; then
  echo "release verifier accepted a missing torrctl binary" >&2
  exit 1
fi
mv "$missing.bak" "$missing"

if FIXTURE_VERSION="$version" FIXTURE_COMMIT="$commit" WRONG_BINARY="${names[0]}" GO_CMD="$fake_go" \
  "$script" "$release" "$version" "$commit" >/dev/null 2>&1; then
  echo "release verifier accepted mismatched metadata" >&2
  exit 1
fi

if FIXTURE_VERSION="$version" FIXTURE_COMMIT="$commit" WRONG_PACKAGE_BINARY="${names[5]}" GO_CMD="$fake_go" \
  "$script" "$release" "$version" "$commit" >/dev/null 2>&1; then
  echo "release verifier accepted a binary built from the wrong package" >&2
  exit 1
fi

echo "release binary verification tests passed"
