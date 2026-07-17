#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
script="$root/.github/scripts/build-release-binary.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

repo="$tmp/repo"
mkdir -p "$repo/.github/scripts" "$repo/server/cmd/torrserver" "$repo/server/cmd/torrctl"
cp "$script" "$root/.github/scripts/build-metadata.sh" "$repo/.github/scripts/"
printf 'package main\n' > "$repo/server/cmd/torrserver/main.go"
printf 'package main\n' > "$repo/server/cmd/torrctl/main.go"

git -C "$repo" init -q
git -C "$repo" config user.name test
git -C "$repo" config user.email test@example.invalid
git -C "$repo" add .
git -C "$repo" commit -qm initial
commit="$(git -C "$repo" rev-parse HEAD)"

fake_go="$tmp/fake-go"
cat > "$fake_go" <<'FAKE_GO'
#!/usr/bin/env bash
set -euo pipefail

output=""
package=""
ldflags=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o)
      output="$2"
      shift 2
      ;;
    -ldflags)
      ldflags="$2"
      shift 2
      ;;
    ./cmd/*)
      package="$1"
      shift
      ;;
    *)
      shift
      ;;
  esac
done

[[ -n "$output" && -n "$package" && -n "$ldflags" ]]
if [[ "${FAIL_PACKAGE:-}" == "$package" ]]; then
  exit 9
fi
printf '%s\t%s\t%s\t%s\n' "$GOOS" "$GOARCH" "$package" "$ldflags" >> "$BUILD_LOG"
printf '%s/%s %s\n' "$GOOS" "$GOARCH" "$package" > "$output"
FAKE_GO
chmod +x "$fake_go"

version="1.0.0-beta.4"
output="$tmp/output"
log="$tmp/build.log"
BUILD_LOG="$log" GO_CMD="$fake_go" "$repo/.github/scripts/build-release-binary.sh" \
  "$repo" "$output" "$version" "$commit" linux amd64 >/dev/null

expected=$'torrctl-1.0.0-beta.4-linux-amd64\ntorrserver-1.0.0-beta.4-linux-amd64'
actual="$(find "$output" -maxdepth 1 -type f -exec basename {} \; | sort)"
[[ "$actual" == "$expected" ]] || { printf 'release outputs differ:\n%s\n' "$actual" >&2; exit 1; }
grep -Fq $'linux\tamd64\t./cmd/torrserver\t' "$log"
grep -Fq $'linux\tamd64\t./cmd/torrctl\t' "$log"
[[ "$(sort -u -t $'\t' -k4,4 "$log" | wc -l | tr -d ' ')" -eq 1 ]] || {
  echo "release binaries received different metadata" >&2
  exit 1
}

if BUILD_LOG="$log" GO_CMD="$fake_go" "$repo/.github/scripts/build-release-binary.sh" \
  "$repo" "$tmp/unsupported" "$version" "$commit" plan9 amd64 >/dev/null 2>&1; then
  echo "release builder accepted an unsupported target" >&2
  exit 1
fi

if BUILD_LOG="$log" GO_CMD="$fake_go" FAIL_PACKAGE=./cmd/torrctl \
  "$repo/.github/scripts/build-release-binary.sh" \
  "$repo" "$tmp/partial" "$version" "$commit" linux amd64 >/dev/null 2>&1; then
  echo "release builder accepted a missing torrctl build" >&2
  exit 1
fi

printf 'dirty\n' > "$repo/untracked.txt"
if BUILD_LOG="$log" GO_CMD="$fake_go" "$repo/.github/scripts/build-release-binary.sh" \
  "$repo" "$tmp/dirty" "$version" "$commit" linux amd64 >/dev/null 2>&1; then
  echo "release builder accepted a dirty worktree" >&2
  exit 1
fi

echo "release binary build tests passed"
