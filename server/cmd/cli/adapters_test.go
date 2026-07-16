package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCommandContextCancellationStopsRequest(t *testing.T) {
	t.Parallel()

	requestStarted := make(chan struct{})
	requestCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		<-request.Context().Done()
		close(requestCanceled)
	}))
	t.Cleanup(server.Close)

	command := newStatusCmd(&globalOptions{Server: server.URL, Timeout: time.Minute, Output: outputTable})
	ctx, cancel := context.WithCancel(context.Background())
	command.SetContext(ctx)
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})

	result := make(chan error, 1)

	go func() {
		result <- command.Execute()
	}()

	<-requestStarted
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("command error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("command did not stop after its context was canceled")
	}

	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("HTTP request context was not canceled")
	}
}

func TestRunWithClientUsesInjectedPasswordPrompt(t *testing.T) {
	t.Parallel()

	const (
		username = "admin"
		password = "test-password"
	)

	var promptCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, pass, ok := request.BasicAuth()
		if !ok || user != username || pass != password {
			t.Errorf("basic auth = (%q, %q, %t)", user, pass, ok)
		}

		writer.Header().Set("Content-Type", "application/json")

		if request.URL.Path == "/readyz" {
			if err := json.NewEncoder(writer).Encode(map[string]any{"status": "ok"}); err != nil {
				t.Errorf("encode readiness response: %v", err)
			}

			return
		}

		if err := json.NewEncoder(writer).Encode(map[string]any{"current": "test"}); err != nil {
			t.Errorf("encode version response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	opts := &globalOptions{
		Server:     server.URL,
		User:       username,
		Timeout:    time.Second,
		Output:     outputTable,
		isTerminal: func() bool { return true },
		readPassword: func(_ io.Writer) (string, error) {
			promptCalls.Add(1)

			return password, nil
		},
	}
	command := newStatusCmd(opts)
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute status: %v", err)
	}

	if promptCalls.Load() != 1 {
		t.Fatalf("password prompt calls = %d, want 1", promptCalls.Load())
	}
}

func TestTorrentHashCompatibilityFlagIsParsedByCobra(t *testing.T) {
	t.Parallel()

	var getRequests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload struct {
			Action string `json:"action"`
		}

		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode torrent request: %v", err)

			return
		}

		writer.Header().Set("Content-Type", "application/json")

		switch payload.Action {
		case "list":
			if _, err := writer.Write([]byte(`[{"hash":"` + testTorrentHash + `","title":"Movie"}]`)); err != nil {
				t.Errorf("write torrent list: %v", err)
			}
		case "get":
			getRequests.Add(1)

			if _, err := writer.Write([]byte(`{"hash":"` + testTorrentHash + `"}`)); err != nil {
				t.Errorf("write torrent status: %v", err)
			}
		default:
			t.Errorf("unexpected action %q", payload.Action)
		}
	}))
	t.Cleanup(server.Close)

	opts := &globalOptions{Server: server.URL, Timeout: time.Second, Output: outputJSON}
	command := newTorrentsCmd(opts)
	command.SetArgs([]string{"get", "--hash", testTorrentHash})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute torrents get --hash: %v", err)
	}

	if getRequests.Load() != 1 {
		t.Fatalf("get requests = %d, want 1", getRequests.Load())
	}

	conflict := newTorrentsCmd(opts)
	conflict.SetArgs([]string{"get", "1", "--hash", testTorrentHash})

	err := conflict.Execute()
	if err == nil || err.Error() != "provide the torrent identifier either positionally or with --hash, not both" {
		t.Fatalf("conflicting identifier error = %v", err)
	}
}
