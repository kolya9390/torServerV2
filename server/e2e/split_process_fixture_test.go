//go:build e2e

package e2e

import (
	"bytes"
	"crypto/sha1" // #nosec G505 -- BitTorrent v1 fixture requires SHA-1 piece hashes.
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

const (
	fixtureDirectory  = "e2e-fixture"
	fixtureMovie      = "movie.mkv"
	fixtureReadme     = "readme.txt"
	fixtureMovieSize  = int64(32)
	fixtureReadmeSize = int64(1)
	fixtureMagnetHash = "0123456789abcdef0123456789abcdef01234567"
)

type jsonEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Status  int    `json:"status"`
	} `json:"error"`
}

type torrentView struct {
	Title      string     `json:"title"`
	Name       string     `json:"name"`
	Hash       string     `json:"hash"`
	StatString string     `json:"stat_string"`
	FileStats  []fileView `json:"file_stats"`
}

type fileView struct {
	ID     int    `json:"id"`
	Path   string `json:"path"`
	Length int64  `json:"length"`
}

type streamURLView struct {
	URL         string `json:"url"`
	TorrentHash string `json:"torrent_hash"`
	FileID      int    `json:"file_id"`
}

func writeFFProbeSentinel(t *testing.T, directory string) {
	t.Helper()

	// DLNA disables probing in this scenario, but its upstream package resolves
	// ffprobe during init. Keep the process test independent of the host image and
	// fail loudly if the disabled probe is ever invoked.
	contents := "#!/bin/sh\necho 'unexpected ffprobe invocation in split-process E2E' >&2\nexit 127\n"
	path := filepath.Join(directory, "ffprobe")
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatalf("write ffprobe sentinel: %v", err)
	}
}

func writeTorrentFixture(t *testing.T, path string) string {
	t.Helper()

	payload := append(bytes.Repeat([]byte{'R'}, int(fixtureReadmeSize)), bytes.Repeat([]byte{'M'}, int(fixtureMovieSize))...)
	pieceHash := sha1.Sum(payload) // #nosec G401 -- protocol compatibility, not security.
	private := true
	infoBytes, err := bencode.Marshal(metainfo.Info{
		PieceLength: 16 * 1024,
		Pieces:      pieceHash[:],
		Name:        fixtureDirectory,
		Private:     &private,
		Files: []metainfo.FileInfo{
			{Path: []string{fixtureReadme}, Length: fixtureReadmeSize},
			{Path: []string{fixtureMovie}, Length: fixtureMovieSize},
		},
	})
	if err != nil {
		t.Fatalf("marshal torrent fixture: %v", err)
	}

	meta := metainfo.MetaInfo{InfoBytes: infoBytes}

	var torrent bytes.Buffer
	if err := meta.Write(&torrent); err != nil {
		t.Fatalf("encode torrent fixture: %v", err)
	}

	if err := os.WriteFile(path, torrent.Bytes(), 0o600); err != nil {
		t.Fatalf("write torrent fixture: %v", err)
	}

	return meta.HashInfoBytes().HexString()
}

func writeReleaseConfig(t *testing.T, path, statePath string, port int) {
	t.Helper()

	contents := fmt.Sprintf(`server:
  port: %q
  shutdown_mode: public
dlna:
  enabled: false
cache:
  size_mb: 16
  preload_percent: 1
  use_disk: false
  torrents_save_path: %q
torrent:
  retrackers_mode: 2
  disconnect_timeout_sec: 5
  connections_limit: 5
network:
  enable_ipv6: false
  disable_upnp: true
  disable_dht: true
  disable_pex: true
  disable_upload: true
  enable_lpd: false
  peers_listen_port: 0
search:
  enable_rutor: false
  enable_torznab: false
debug:
  enabled: false
storage:
  settings_in_json: true
  viewed_in_json: true
`, fmt.Sprintf("%d", port), filepath.Join(statePath, "downloads"))

	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write release config: %v", err)
	}
}

func decodeSuccess[T any](t *testing.T, result processResult) T {
	t.Helper()

	if result.timedOut || result.exitCode != 0 {
		t.Fatalf(
			"command failed: exit=%d timeout=%v stdout=%s stderr=%s",
			result.exitCode,
			result.timedOut,
			result.stdout,
			result.stderr,
		)
	}

	var envelope jsonEnvelope
	if err := json.Unmarshal([]byte(result.stdout), &envelope); err != nil {
		t.Fatalf("decode JSON success envelope: %v\n%s", err, result.stdout)
	}

	if !envelope.OK {
		t.Fatalf("JSON envelope reports failure: %s", result.stdout)
	}

	var data T
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatalf("decode JSON data: %v\n%s", err, result.stdout)
	}

	return data
}

func decodeFailure(t *testing.T, result processResult) jsonEnvelope {
	t.Helper()

	if result.timedOut || result.exitCode == 0 {
		t.Fatalf(
			"command unexpectedly succeeded or timed out: exit=%d timeout=%v stdout=%s stderr=%s",
			result.exitCode,
			result.timedOut,
			result.stdout,
			result.stderr,
		)
	}

	var envelope jsonEnvelope
	if err := json.Unmarshal([]byte(result.stderr), &envelope); err != nil {
		t.Fatalf("decode JSON error envelope: %v\n%s", err, result.stderr)
	}

	if envelope.OK || strings.TrimSpace(envelope.Error.Code) == "" || strings.TrimSpace(envelope.Error.Message) == "" {
		t.Fatalf("invalid JSON error envelope: %s", result.stderr)
	}

	return envelope
}

func assertStreamURL(t *testing.T, stream streamURLView, baseURL, hash string, fileID int) {
	t.Helper()

	if stream.TorrentHash != hash || stream.FileID != fileID {
		t.Fatalf("stream selection = %+v, want hash=%s file=%d", stream, hash, fileID)
	}

	parsed, err := url.Parse(stream.URL)
	if err != nil {
		t.Fatalf("parse stream URL: %v", err)
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}

	if parsed.Scheme != base.Scheme || parsed.Host != base.Host || parsed.Path != "/streams/play" {
		t.Fatalf("stream URL target = %s, want %s/streams/play", stream.URL, baseURL)
	}

	if parsed.Query().Get("link") != hash || parsed.Query().Get("index") != fmt.Sprintf("%d", fileID) {
		t.Fatalf("stream URL query = %s", parsed.RawQuery)
	}
}

func assertDirectoryEmpty(t *testing.T, path string) {
	t.Helper()

	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatalf("read side-effect directory: %v", err)
	}

	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}

		t.Fatalf("unexpected filesystem side effects in %s: %v", path, names)
	}
}

func assertRegularFile(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat required file %s: %v", path, err)
	}

	if !info.Mode().IsRegular() {
		t.Fatalf("required path %s has mode %s, want regular file", path, info.Mode())
	}
}

func assertTopLevelEntries(t *testing.T, root string, allowed ...string) {
	t.Helper()

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read E2E root: %v", err)
	}

	want := append([]string(nil), allowed...)
	sort.Strings(want)
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	sort.Strings(got)

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("E2E root entries = %v, want %v", got, want)
	}
}
