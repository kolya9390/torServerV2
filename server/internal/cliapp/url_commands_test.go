package cliapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitForTorrentFilesPollsUntilMetadataIsReady(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/torrents" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		call := calls.Add(1)

		w.Header().Set("Content-Type", "application/json")

		if call < 3 {
			_, _ = w.Write([]byte(`{"file_stats":[]}`))

			return
		}

		_, _ = w.Write([]byte(`{"file_stats":[{"id":7,"length":4096,"path":"movie.mkv"}]}`))
	}))
	defer server.Close()

	client := newTestAPIClient(t, server.URL, "", "")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	files, err := waitForTorrentFiles(ctx, client, testTorrentHash)
	if err != nil {
		t.Fatalf("wait for files: %v", err)
	}

	if len(files) != 1 || files[0].ID != 7 || files[0].Path != "movie.mkv" {
		t.Fatalf("files = %#v, want movie file", files)
	}

	if got := calls.Load(); got != 3 {
		t.Fatalf("request count = %d, want 3", got)
	}
}

func TestWaitForTorrentFilesHonorsContextDeadline(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"file_stats":null}`))
	}))
	defer server.Close()

	client := newTestAPIClient(t, server.URL, "", "")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := waitForTorrentFiles(ctx, client, testTorrentHash)
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
}

func TestWaitForTorrentFilesRejectsMalformedFileList(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"file_stats":{"id":1}}`))
	}))
	defer server.Close()

	client := newTestAPIClient(t, server.URL, "", "")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := waitForTorrentFiles(ctx, client, testTorrentHash)
	if err == nil || !strings.Contains(err.Error(), "parse torrent file list") {
		t.Fatalf("error = %v, want malformed file list error", err)
	}
}

func TestWaitForTorrentFilesReturnsPersistedFileList(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"stat_string":"Torrent in db",
			"data":"{\"TorrServer\":{\"Files\":[{\"id\":2,\"length\":2048,\"path\":\"movie.mkv\"}]}}"
		}`))
	}))
	defer server.Close()

	client := newTestAPIClient(t, server.URL, "", "")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	files, err := waitForTorrentFiles(ctx, client, testTorrentHash)
	if err != nil {
		t.Fatalf("wait for persisted files: %v", err)
	}

	if len(files) != 1 || files[0].ID != 2 {
		t.Fatalf("files = %#v, want persisted file ID 2", files)
	}
}

func TestWaitForTorrentFilesRejectsStoredTorrentWithoutFileList(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stat_string":"Torrent in db","data":"{\"lampa\":true}"}`))
	}))
	defer server.Close()

	client := newTestAPIClient(t, server.URL, "", "")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := waitForTorrentFiles(ctx, client, testTorrentHash)
	if !errors.Is(err, errTorrentStoredOnly) {
		t.Fatalf("error = %v, want stored-only error", err)
	}
}

func TestWaitForTorrentFilesSendsGetAction(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		if request["action"] != "get" || request["hash"] != testTorrentHash {
			t.Fatalf("request = %#v, want get for test hash", request)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"file_stats":[{"id":1,"length":1024,"path":"movie.mkv"}]}`))
	}))
	defer server.Close()

	client := newTestAPIClient(t, server.URL, "", "")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := waitForTorrentFiles(ctx, client, testTorrentHash); err != nil {
		t.Fatalf("wait for files: %v", err)
	}
}

func TestTorrentAddedMessageShowsExactNextCommand(t *testing.T) {
	t.Parallel()

	out := torrentStatus{Title: "Movie", Hash: testTorrentHash}

	tests := []struct {
		name string
		opts globalOptions
		want string
	}{
		{
			name: "default server",
			want: "Next: torrserver url " + testTorrentHash,
		},
		{
			name: "context",
			opts: globalOptions{Context: "home", Server: "http://192.0.2.10:8090"},
			want: "Next: torrserver --context home url " + testTorrentHash,
		},
		{
			name: "explicit server",
			opts: globalOptions{Server: "http://192.0.2.10:8090"},
			want: "Next: torrserver --server http://192.0.2.10:8090 url " + testTorrentHash,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			message := torrentAddedMessage(out, test.opts)
			if !strings.Contains(message, "Added: Movie") || !strings.Contains(message, test.want) {
				t.Fatalf("message = %q, want add summary and %q", message, test.want)
			}
		})
	}
}

func TestBuildStreamURL(t *testing.T) {
	t.Parallel()

	got := buildStreamURL("http://192.0.2.10:8090/base", testTorrentHash, 7)
	want := "http://192.0.2.10:8090/streams/play?index=7&link=" + testTorrentHash
	if got != want {
		t.Fatalf("stream URL = %q, want %q", got, want)
	}
}

func TestSanitizeTerminalTextRemovesControlCharacters(t *testing.T) {
	t.Parallel()

	got := sanitizeTerminalText("movie\x1b[31m\n.mkv")
	if got != "movie?[31m?.mkv" {
		t.Fatalf("sanitized text = %q", got)
	}
}

func TestURLWithNumericFileBuildsLinkWithoutMetadataRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		if request["action"] != "list" {
			t.Fatalf("unexpected action %v; numeric --file must not fetch metadata", request["action"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"hash":"` + testTorrentHash + `","title":"Movie"}]`))
	}))
	defer server.Close()

	client := newTestAPIClient(t, server.URL, "", "")
	opts := globalOptions{Server: server.URL, Timeout: time.Second}
	if err := cmdURLWithFlags(client, opts, "1", false, "3"); err != nil {
		t.Fatalf("build URL for numeric file: %v", err)
	}
}
