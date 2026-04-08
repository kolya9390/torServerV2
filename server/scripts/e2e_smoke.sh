#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARTIFACT_DIR="${E2E_ARTIFACT_DIR:-${ROOT_DIR}/artifacts/e2e-smoke}"
PORT="${E2E_PORT:-$((20000 + (RANDOM % 20000)))}"
HOST="127.0.0.1"
BASE_URL="http://${HOST}:${PORT}"
RUNTIME_DIR="${E2E_RUNTIME_DIR:-$(mktemp -d)}"
SERVER_LOG="${ARTIFACT_DIR}/server.log"

mkdir -p "${ARTIFACT_DIR}"

cleanup() {
  local code=$?
  if [[ -n "${SERVER_PID:-}" ]]; then
    kill "${SERVER_PID}" >/dev/null 2>&1 || true
    wait "${SERVER_PID}" >/dev/null 2>&1 || true
  fi
  if [[ ${code} -ne 0 ]]; then
    echo "[e2e] failed, see artifacts in: ${ARTIFACT_DIR}" >&2
  fi
  return ${code}
}
trap cleanup EXIT

wait_for_server() {
  local attempts=60
  for _ in $(seq 1 "${attempts}"); do
    if curl -fsS "${BASE_URL}/echo" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  echo "[e2e] server did not become ready at ${BASE_URL}" >&2
  return 1
}

json_get_field() {
  local file="$1"
  local field="$2"
  python3 - "${file}" "${field}" <<'PY'
import json,sys
obj=json.load(open(sys.argv[1], encoding='utf-8'))
value=obj
for key in sys.argv[2].split('.'):
    if isinstance(value, dict):
        value=value.get(key)
    else:
        value=None
        break
if value is None:
    print("")
elif isinstance(value, bool):
    print("true" if value else "false")
else:
    print(value)
PY
}

json_array_contains_hash() {
  local file="$1"
  local expected_hash="$2"
  python3 - "${file}" "${expected_hash}" <<'PY'
import json,sys
arr=json.load(open(sys.argv[1], encoding='utf-8'))
expected=sys.argv[2]
print("yes" if any(isinstance(i, dict) and i.get('hash')==expected for i in arr) else "no")
PY
}

request_json() {
  local method="$1"
  local url="$2"
  local body="${3:-}"
  local out_file="$4"
  local code_file="$5"

  if [[ -n "${body}" ]]; then
    curl -sS -X "${method}" -H 'content-type: application/json' \
      -d "${body}" -o "${out_file}" -w '%{http_code}' "${url}" > "${code_file}"
  else
    curl -sS -X "${method}" -o "${out_file}" -w '%{http_code}' "${url}" > "${code_file}"
  fi
}

assert_http() {
  local code_file="$1"
  shift
  local code
  code="$(cat "${code_file}")"
  for allowed in "$@"; do
    if [[ "${code}" == "${allowed}" ]]; then
      return 0
    fi
  done
  echo "[e2e] unexpected HTTP status: ${code}, expected one of: $*" >&2
  return 1
}

build_fixture_torrent() {
  local payload_file="$1"
  local torrent_file="$2"
  local helper_go="${ARTIFACT_DIR}/_make_torrent.go"

  cat > "${helper_go}" <<'GO'
package main

import (
	"os"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

func main() {
	payload := os.Args[1]
	torrentOut := os.Args[2]
	_ = os.WriteFile(payload, []byte("e2e smoke payload"), 0o644)

	info := metainfo.Info{PieceLength: 16 * 1024, Name: "payload.bin"}
	if err := info.BuildFromFilePath(payload); err != nil {
		panic(err)
	}
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		panic(err)
	}
	mi := metainfo.MetaInfo{InfoBytes: infoBytes, Announce: "http://127.0.0.1:1/announce"}
	f, err := os.Create(torrentOut)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := mi.Write(f); err != nil {
		panic(err)
	}
}
GO

  GOCACHE=/tmp/go-build-cache go run "${helper_go}" "${payload_file}" "${torrent_file}"
}

start_server() {
  (cd "${ROOT_DIR}" && GOCACHE=/tmp/go-build-cache go run ./cmd --port "${PORT}" --path "${RUNTIME_DIR}" --torrentsdir "${RUNTIME_DIR}") > "${SERVER_LOG}" 2>&1 &
  SERVER_PID=$!
  wait_for_server
}

restart_server() {
  if [[ -n "${SERVER_PID:-}" ]]; then
    kill "${SERVER_PID}" >/dev/null 2>&1 || true
    wait "${SERVER_PID}" >/dev/null 2>&1 || true
  fi
  start_server
}

main() {
  echo "[e2e] runtime dir: ${RUNTIME_DIR}"
  echo "[e2e] artifacts dir: ${ARTIFACT_DIR}"

  start_server

  local payload_file="${ARTIFACT_DIR}/payload.bin"
  local torrent_file="${ARTIFACT_DIR}/fixture.torrent"
  build_fixture_torrent "${payload_file}" "${torrent_file}"

  local file_link="file://${torrent_file}"
  local encoded_link
  encoded_link="$(python3 - <<'PY' "${file_link}"
import urllib.parse,sys
print(urllib.parse.quote(sys.argv[1], safe=''))
PY
)"

  # 1) settings get/set/apply
  request_json POST "${BASE_URL}/settings" '{"action":"get"}' "${ARTIFACT_DIR}/settings.get.json" "${ARTIFACT_DIR}/settings.get.code"
  assert_http "${ARTIFACT_DIR}/settings.get.code" 200

  request_json POST "${BASE_URL}/settings" '{"action":"set","sets":{"friendlyName":"TS-E2E","enableRutorSearch":false}}' "${ARTIFACT_DIR}/settings.set.json" "${ARTIFACT_DIR}/settings.set.code"
  assert_http "${ARTIFACT_DIR}/settings.set.code" 200

  request_json POST "${BASE_URL}/settings" '{"action":"get"}' "${ARTIFACT_DIR}/settings.after-set.json" "${ARTIFACT_DIR}/settings.after-set.code"
  assert_http "${ARTIFACT_DIR}/settings.after-set.code" 200
  if [[ "$(json_get_field "${ARTIFACT_DIR}/settings.after-set.json" friendlyName)" != "TS-E2E" ]]; then
    echo "[e2e] settings apply check failed: friendlyName mismatch" >&2
    return 1
  fi

  # 2) add torrent
  request_json POST "${BASE_URL}/torrents" "{\"action\":\"add\",\"link\":\"${file_link}\",\"save_to_db\":true}" "${ARTIFACT_DIR}/torrent.add.json" "${ARTIFACT_DIR}/torrent.add.code"
  assert_http "${ARTIFACT_DIR}/torrent.add.code" 200

  local hash
  hash="$(json_get_field "${ARTIFACT_DIR}/torrent.add.json" hash)"
  if [[ -z "${hash}" ]]; then
    echo "[e2e] add torrent did not return hash" >&2
    return 1
  fi

  # 3) preload + playback route smoke (validation-level)
  request_json GET "${BASE_URL}/streams/play?link=${encoded_link}&index=999&preload=1" "" "${ARTIFACT_DIR}/stream.preload-play.json" "${ARTIFACT_DIR}/stream.preload-play.code"
  assert_http "${ARTIFACT_DIR}/stream.preload-play.code" 400

  # 4) save torrent metadata
  request_json POST "${BASE_URL}/streams/save?link=${encoded_link}&title=TS-E2E-SAVE" "" "${ARTIFACT_DIR}/stream.save.json" "${ARTIFACT_DIR}/stream.save.code"
  assert_http "${ARTIFACT_DIR}/stream.save.code" 200

  # 5) verify list contains torrent before restart
  request_json POST "${BASE_URL}/torrents" '{"action":"list"}' "${ARTIFACT_DIR}/torrent.list.before-restart.json" "${ARTIFACT_DIR}/torrent.list.before-restart.code"
  assert_http "${ARTIFACT_DIR}/torrent.list.before-restart.code" 200
  if [[ "$(json_array_contains_hash "${ARTIFACT_DIR}/torrent.list.before-restart.json" "${hash}")" != "yes" ]]; then
    echo "[e2e] list before restart does not contain saved torrent" >&2
    return 1
  fi

  # 6) restart and verify recovery
  restart_server
  request_json POST "${BASE_URL}/settings" '{"action":"get"}' "${ARTIFACT_DIR}/settings.after-restart.json" "${ARTIFACT_DIR}/settings.after-restart.code"
  assert_http "${ARTIFACT_DIR}/settings.after-restart.code" 200
  if [[ "$(json_get_field "${ARTIFACT_DIR}/settings.after-restart.json" friendlyName)" != "TS-E2E" ]]; then
    echo "[e2e] settings recovery failed after restart" >&2
    return 1
  fi

  request_json POST "${BASE_URL}/torrents" '{"action":"list"}' "${ARTIFACT_DIR}/torrent.list.after-restart.json" "${ARTIFACT_DIR}/torrent.list.after-restart.code"
  assert_http "${ARTIFACT_DIR}/torrent.list.after-restart.code" 200
  if [[ "$(json_array_contains_hash "${ARTIFACT_DIR}/torrent.list.after-restart.json" "${hash}")" != "yes" ]]; then
    echo "[e2e] torrent recovery failed after restart" >&2
    return 1
  fi

  # 7) remove and verify cleanup
  request_json POST "${BASE_URL}/torrents" "{\"action\":\"rem\",\"hash\":\"${hash}\"}" "${ARTIFACT_DIR}/torrent.rem.json" "${ARTIFACT_DIR}/torrent.rem.code"
  assert_http "${ARTIFACT_DIR}/torrent.rem.code" 200

  request_json POST "${BASE_URL}/torrents" '{"action":"list"}' "${ARTIFACT_DIR}/torrent.list.after-rem.json" "${ARTIFACT_DIR}/torrent.list.after-rem.code"
  assert_http "${ARTIFACT_DIR}/torrent.list.after-rem.code" 200
  if [[ "$(json_array_contains_hash "${ARTIFACT_DIR}/torrent.list.after-rem.json" "${hash}")" != "no" ]]; then
    echo "[e2e] remove torrent check failed" >&2
    return 1
  fi

  echo "[e2e] smoke scenario passed"
}

main "$@"
