package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestConfirmDestructiveAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		yes         bool
		interactive bool
		cancel      bool
		wantErr     error
		wantText    string
		wantPrompt  bool
	}{
		{name: "confirmed", input: "yes\n", interactive: true, wantPrompt: true},
		{name: "confirmed case insensitive", input: " YES \n", interactive: true, wantPrompt: true},
		{name: "declined", input: "no\n", interactive: true, wantErr: errOperationCanceled, wantPrompt: true},
		{name: "empty input cancels", interactive: true, wantErr: errOperationCanceled, wantPrompt: true},
		{name: "yes bypasses non interactive prompt", yes: true},
		{name: "non interactive fails", wantText: "rerun with --yes"},
		{name: "canceled context", input: "yes\n", interactive: true, cancel: true, wantText: "context canceled"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			if test.cancel {
				canceled, cancel := context.WithCancel(ctx)
				cancel()

				ctx = canceled
			}

			var output bytes.Buffer

			err := confirmDestructiveAction(ctx, confirmationRequest{
				Action:      "Delete all torrents",
				Yes:         test.yes,
				Interactive: test.interactive,
				Input:       strings.NewReader(test.input),
				Output:      &output,
			})

			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}

			if test.wantText != "" && (err == nil || !strings.Contains(err.Error(), test.wantText)) {
				t.Fatalf("error = %v, want text %q", err, test.wantText)
			}

			if test.wantErr == nil && test.wantText == "" && err != nil {
				t.Fatalf("confirmation error: %v", err)
			}

			if got := output.Len() > 0; got != test.wantPrompt {
				t.Fatalf("prompt written = %t, want %t", got, test.wantPrompt)
			}
		})
	}
}

func TestDestructiveCommandsDoNotPromptNonInteractiveInput(t *testing.T) {
	var requests atomic.Int32
	server := newDestructiveCommandServer(t, &requests)
	defer server.Close()

	configureTestContextPath(t)

	tests := []struct {
		name    string
		command func(*globalOptions) *cobra.Command
		args    []string
	}{
		{name: "wipe", command: newTorrentsCmd, args: []string{"wipe"}},
		{name: "settings reset", command: newSettingsCmd, args: []string{"def"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := requests.Load()
			cmd := test.command(&globalOptions{Server: server.URL, Timeout: time.Second, Output: outputTable})
			cmd.SetArgs(test.args)
			cmd.SetIn(strings.NewReader("yes\n"))
			cmd.SetErr(&bytes.Buffer{})

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "--yes") {
				t.Fatalf("error = %v, want --yes guidance", err)
			}

			if got := requests.Load(); got != before {
				t.Fatalf("request count changed from %d to %d", before, got)
			}
		})
	}
}

func TestDestructiveCommandsYesFlagSendsOneRequest(t *testing.T) {
	var requests atomic.Int32
	server := newDestructiveCommandServer(t, &requests)
	defer server.Close()

	configureTestContextPath(t)

	tests := []struct {
		name    string
		command func(*globalOptions) *cobra.Command
		args    []string
	}{
		{name: "wipe", command: newTorrentsCmd, args: []string{"wipe", "--yes"}},
		{name: "settings reset", command: newSettingsCmd, args: []string{"def", "--yes"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := requests.Load()
			cmd := test.command(&globalOptions{Server: server.URL, Timeout: time.Second, Output: outputTable})
			cmd.SetArgs(test.args)
			cmd.SetIn(strings.NewReader(""))
			cmd.SetErr(&bytes.Buffer{})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute with --yes: %v", err)
			}

			if got := requests.Load(); got != before+1 {
				t.Fatalf("request count = %d, want %d", got, before+1)
			}
		})
	}
}

func newDestructiveCommandServer(t *testing.T, requests *atomic.Int32) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var payload struct {
			Action string `json:"action"`
		}

		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode destructive request: %v", err)

			return
		}

		if payload.Action != "wipe" && payload.Action != "def" {
			t.Errorf("unexpected destructive action %q", payload.Action)

			return
		}

		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
}

func configureTestContextPath(t *testing.T) {
	t.Helper()
	t.Setenv(envConfig, filepath.Join(t.TempDir(), "contexts.json"))
}
