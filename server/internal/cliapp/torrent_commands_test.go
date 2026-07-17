package cliapp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"server/internal/apiclient"
)

const testTorrentHash = "0123456789abcdef0123456789abcdef01234567"

func TestCanonicalTorrentHash(t *testing.T) {
	t.Parallel()

	upper := strings.ToUpper(testTorrentHash)
	if got, err := canonicalTorrentHash(upper); err != nil || got != testTorrentHash {
		t.Fatalf("canonicalTorrentHash(%q) = (%q, %v)", upper, got, err)
	}

	for _, invalid := range []string{"short", strings.Repeat("z", 40)} {
		if _, err := canonicalTorrentHash(invalid); err == nil {
			t.Fatalf("canonicalTorrentHash(%q) unexpectedly succeeded", invalid)
		}
	}
}

func TestResolveTorrentAddOptions(t *testing.T) {
	t.Parallel()

	torrentPath := writeTestTorrentFile(t, "movie.torrent", "torrent metadata")
	fileURL := (&url.URL{Scheme: "file", Path: torrentPath}).String()

	tests := []struct {
		name     string
		args     []string
		opts     torrentAddOptions
		wantFile string
		wantLink string
		wantErr  string
	}{
		{
			name:     "positional magnet",
			args:     []string{"magnet:?xt=urn:btih:" + testTorrentHash},
			wantLink: "magnet:?xt=urn:btih:" + testTorrentHash,
		},
		{name: "link flag", opts: torrentAddOptions{Link: testTorrentHash}, wantLink: testTorrentHash},
		{name: "explicit file", opts: torrentAddOptions{File: torrentPath}, wantFile: torrentPath, wantLink: torrentPath},
		{name: "positional local file", args: []string{torrentPath}, wantFile: torrentPath, wantLink: torrentPath},
		{name: "file URI", args: []string{fileURL}, wantFile: torrentPath, wantLink: fileURL},
		{
			name:     "remote torrent URL",
			args:     []string{"https://example.test/movie.torrent"},
			wantLink: "https://example.test/movie.torrent",
		},
		{name: "missing source", wantErr: "requires a magnet"},
		{
			name:    "conflicting sources",
			args:    []string{testTorrentHash},
			opts:    torrentAddOptions{Link: "magnet:?xt=urn:btih:" + testTorrentHash},
			wantErr: "exactly one torrent source",
		},
		{name: "missing local file", args: []string{"missing.torrent"}, wantErr: "does not exist"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveTorrentAddOptions(test.args, test.opts)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, test.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("resolve options: %v", err)
			}

			if got.File != test.wantFile {
				t.Fatalf("file = %q, want %q", got.File, test.wantFile)
			}

			if got.Source != test.wantLink {
				t.Fatalf("source = %q, want %q", got.Source, test.wantLink)
			}
		})
	}
}

func TestResolveTorrentAddOptionsRejectsInvalidFiles(t *testing.T) {
	t.Parallel()

	emptyPath := writeTestTorrentFile(t, "empty.torrent", "")
	largePath := filepath.Join(t.TempDir(), "large.torrent")
	if err := os.WriteFile(largePath, []byte{1}, 0o600); err != nil {
		t.Fatalf("write large torrent fixture: %v", err)
	}

	if err := os.Truncate(largePath, maxTorrentUploadFileBytes); err != nil {
		t.Fatalf("truncate large torrent fixture: %v", err)
	}

	directory := t.TempDir()

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "empty", path: emptyPath, wantErr: "is empty"},
		{name: "too large", path: largePath, wantErr: "too large"},
		{name: "directory", path: directory, wantErr: "not a regular file"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := resolveTorrentAddOptions(nil, torrentAddOptions{File: test.path})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestCmdTorrentsAddSendsJSONForMagnet(t *testing.T) {
	t.Parallel()

	var request map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/torrents" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}

		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		writeTorrentAddResponse(t, w)
	}))
	defer server.Close()

	client := newTestAPIClient(t, server.URL, "", "")
	opts := globalOptions{Server: server.URL, Timeout: time.Second, Output: outputJSON}
	addOpts := torrentAddOptions{
		Source: "magnet:?xt=urn:btih:" + testTorrentHash,
		Title:  "Movie",
		Save:   true,
	}

	if err := cmdTorrentsAdd(client, opts, addOpts); err != nil {
		t.Fatalf("add magnet: %v", err)
	}

	if got := request["action"]; got != "add" {
		t.Fatalf("action = %v, want add", got)
	}

	if got := request["link"]; got != addOpts.Source {
		t.Fatalf("link = %v, want %q", got, addOpts.Source)
	}

	if got := request["save_to_db"]; got != true {
		t.Fatalf("save_to_db = %v, want true", got)
	}
}

func TestCmdTorrentsAddUploadsLocalFile(t *testing.T) {
	t.Parallel()

	const (
		user     = "cli-user"
		password = "cli-pass"
		content  = "torrent metadata bytes"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/torrent/upload" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}

		gotUser, gotPassword, ok := r.BasicAuth()
		if !ok || gotUser != user || gotPassword != password {
			t.Fatalf("basic auth = (%q, %q, %t), want configured credentials", gotUser, gotPassword, ok)
		}

		if err := r.ParseMultipartForm(maxTorrentUploadFileBytes); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("read file field: %v", err)
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read uploaded file: %v", err)
		}

		if header.Filename != "movie.torrent" || string(data) != content {
			t.Fatalf("uploaded file = (%q, %q)", header.Filename, data)
		}

		for field, want := range map[string]string{
			"title":    "Movie",
			"poster":   "https://example.test/poster.jpg",
			"category": "4K",
			"data":     "custom",
			"save":     "true",
		} {
			if got := r.FormValue(field); got != want {
				t.Fatalf("form field %s = %q, want %q", field, got, want)
			}
		}

		writeTorrentAddResponse(t, w)
	}))
	defer server.Close()

	client := newTestAPIClient(t, server.URL, user, password)
	torrentPath := writeTestTorrentFile(t, "movie.torrent", content)
	opts := globalOptions{Server: server.URL, Timeout: time.Second, Output: outputJSON}
	addOpts := torrentAddOptions{
		File:     torrentPath,
		Title:    "Movie",
		Poster:   "https://example.test/poster.jpg",
		Category: "4K",
		Data:     "custom",
		Save:     true,
	}

	if err := cmdTorrentsAdd(client, opts, addOpts); err != nil {
		t.Fatalf("upload torrent: %v", err)
	}
}

func TestCmdTorrentsAddReturnsUploadAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"type":"validation_error","message":"invalid torrent","field":"file"}}`)
	}))
	defer server.Close()

	client := newTestAPIClient(t, server.URL, "", "")
	torrentPath := writeTestTorrentFile(t, "invalid.torrent", "invalid")
	opts := globalOptions{Server: server.URL, Timeout: time.Second, Output: outputJSON}

	err := cmdTorrentsAdd(client, opts, torrentAddOptions{File: torrentPath})
	if err == nil || !strings.Contains(err.Error(), "file: invalid torrent") {
		t.Fatalf("error = %v, want upload validation error", err)
	}
}

func TestCmdTorrentsAddCancelsUploadOnTimeout(t *testing.T) {
	t.Parallel()

	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-releaseRequest
	}))
	defer server.Close()

	client := newTestAPIClient(t, server.URL, "", "")
	torrentPath := writeTestTorrentFile(t, "movie.torrent", "torrent metadata")
	opts := globalOptions{Server: server.URL, Timeout: 20 * time.Millisecond, Output: outputJSON}

	err := cmdTorrentsAdd(client, opts, torrentAddOptions{File: torrentPath})

	close(releaseRequest)

	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("upload request did not reach test server")
	}
}

func TestTorrentCLIHappyPathListUploadAndStreamURL(t *testing.T) {
	t.Parallel()

	var (
		added   atomic.Bool
		getSeen atomic.Bool
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/v1/torrent/upload":
			if err := r.ParseMultipartForm(maxTorrentUploadFileBytes); err != nil {
				t.Fatalf("parse multipart: %v", err)
			}

			file, _, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("read uploaded torrent: %v", err)
			}

			if err := file.Close(); err != nil {
				t.Fatalf("close uploaded torrent: %v", err)
			}

			added.Store(true)

			writeTorrentAddResponse(t, w)
		case "/api/v1/torrents":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode torrent request: %v", err)
			}

			switch request["action"] {
			case "list":
				if !added.Load() {
					_, _ = w.Write([]byte(`[]`))

					return
				}

				_, _ = w.Write([]byte(`[{"hash":"` + testTorrentHash + `","title":"Movie"}]`))
			case "get":
				getSeen.Store(true)

				_, _ = w.Write([]byte(`{
					"hash":"` + testTorrentHash + `",
					"file_stats":[{"id":4,"length":8192,"path":"Movie.mkv"}]
				}`))
			default:
				t.Fatalf("unexpected torrent action %v", request["action"])
			}
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestAPIClient(t, server.URL, "", "")
	opts := globalOptions{Server: server.URL, Timeout: time.Second, Output: outputTable}

	if err := cmdTorrentsList(client, opts); err != nil {
		t.Fatalf("list before add: %v", err)
	}

	torrentPath := writeTestTorrentFile(t, "movie.torrent", "torrent metadata")
	addOpts, err := resolveTorrentAddOptions([]string{torrentPath}, torrentAddOptions{Save: true})
	if err != nil {
		t.Fatalf("resolve upload: %v", err)
	}

	if err := cmdTorrentsAdd(client, opts, addOpts); err != nil {
		t.Fatalf("upload torrent: %v", err)
	}

	if err := cmdTorrentsList(client, opts); err != nil {
		t.Fatalf("list after add: %v", err)
	}

	if err := cmdURLWithFlags(client, opts, "1", false, ""); err != nil {
		t.Fatalf("build stream URL: %v", err)
	}

	if !added.Load() || !getSeen.Load() {
		t.Fatalf("happy path incomplete: added=%t get_seen=%t", added.Load(), getSeen.Load())
	}
}

func newTestAPIClient(t *testing.T, serverURL, user, password string) *apiClient {
	t.Helper()

	client, err := apiclient.New(apiclient.Options{
		BaseURL:  serverURL,
		User:     user,
		Password: password,
		Timeout:  time.Second,
	})
	if err != nil {
		t.Fatalf("new API client: %v", err)
	}

	return client
}

func writeTestTorrentFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write torrent fixture: %v", err)
	}

	return path
}

func writeTorrentAddResponse(t *testing.T, w http.ResponseWriter) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]any{
		"title": "Movie",
		"hash":  testTorrentHash,
	}); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
