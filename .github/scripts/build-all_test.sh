#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

fake_go="$tmp/fake-go"
cat > "$fake_go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

output=""
package=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o)
      output="$2"
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

[[ -n "$output" && -n "$package" ]]
printf '%s/%s %s\n' "$GOOS" "$GOARCH" "$package" >> "$BUILD_LOG"
printf '%s/%s %s\n' "$GOOS" "$GOARCH" "$package" > "$output"
EOF
chmod +x "$fake_go"

dist="$tmp/dist"
log="$tmp/build.log"
mkdir -p "$dist"
printf 'legacy\n' > "$dist/torrserver"
printf 'legacy\n' > "$dist/torrctl"
printf 'legacy\n' > "$dist/tsctl"

BUILD_LOG="$log" GO_CMD="$fake_go" DIST_DIR="$dist" "$root/build-all.sh" >/dev/null

expected=$'torrctl-darwin-amd64\ntorrctl-darwin-arm64\ntorrctl-linux-amd64\ntorrctl-linux-arm64\ntorrctl-windows-amd64.exe\ntorrserver-darwin-amd64\ntorrserver-darwin-arm64\ntorrserver-linux-amd64\ntorrserver-linux-arm64\ntorrserver-windows-amd64.exe'
actual="$(find "$dist" -maxdepth 1 -type f -exec basename {} \; | sort)"
if [[ "$actual" != "$expected" ]]; then
  printf 'cross-build artifact set mismatch:\n%s\n' "$actual" >&2
  exit 1
fi

[[ "$(wc -l < "$log" | tr -d ' ')" -eq 10 ]]
for artifact in "$dist"/torrserver-* "$dist"/torrctl-*; do
  if [[ "$artifact" != *.exe && ! -x "$artifact" ]]; then
    echo "cross-build artifact is not executable: $artifact" >&2
    exit 1
  fi
done

before_calls="$(wc -l < "$log" | tr -d ' ')"
set +e
BUILD_LOG="$log" GO_CMD="$fake_go" DIST_DIR="$dist" "$root/build-all.sh" plan9/amd64 >/dev/null 2>&1
status=$?
set -e
[[ "$status" -eq 2 ]] || { echo "unsupported target exit = $status, want 2" >&2; exit 1; }
[[ "$(wc -l < "$log" | tr -d ' ')" -eq "$before_calls" ]] || {
  echo "unsupported target invoked the compiler" >&2
  exit 1
}

selective="$tmp/selective"
BUILD_LOG="$log" GO_CMD="$fake_go" DIST_DIR="$selective" "$root/build-all.sh" darwin/arm64 >/dev/null
selective_names="$(find "$selective" -maxdepth 1 -type f -exec basename {} \; | sort)"
[[ "$selective_names" == $'torrctl-darwin-arm64\ntorrserver-darwin-arm64' ]] || {
  printf 'selective artifact set mismatch:\n%s\n' "$selective_names" >&2
  exit 1
}

echo "cross-build helper tests passed"
