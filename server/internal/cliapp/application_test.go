package cliapp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"server/internal/apiclient"
	buildversion "server/version"
)

func TestRunLocalVersionHasNoRemoteOrFilesystemSideEffects(t *testing.T) {
	t.Parallel()

	var clientCalls atomic.Int32
	dependencies := testDependencies(&stubFileSystem{failOnAccess: true})
	dependencies.NewClient = func(apiclient.Options) (*apiclient.Client, error) {
		clientCalls.Add(1)

		return nil, errors.New("client must not be created")
	}
	dependencies.BuildInfo = buildversion.Info{
		Version:   "v1.0.0-test.1",
		Commit:    "0123456789ab",
		BuildTime: "2026-07-16T20:00:00Z",
		GoVersion: "go1.26.3",
		OS:        "darwin",
		Arch:      "arm64",
	}

	var stdout bytes.Buffer
	code := Run(Invocation{
		Args:   []string{"--output=json", "version"},
		Stdout: &stdout,
	}, dependencies)
	if code != 0 {
		t.Fatalf("exit code = %d, output = %s", code, stdout.String())
	}

	if clientCalls.Load() != 0 {
		t.Fatalf("client factory calls = %d, want 0", clientCalls.Load())
	}

	var response struct {
		OK   bool `json:"ok"`
		Data struct {
			Source  string `json:"source"`
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode version output: %v", err)
	}

	if !response.OK || response.Data.Source != localBinaryVersionSource || response.Data.Version != "v1.0.0-test.1" {
		t.Fatalf("version response = %+v", response)
	}
}

func TestRunReturnsDeterministicErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		args        []string
		wantMessage string
	}{
		{
			name:        "invalid output",
			args:        []string{"--output=yaml", "version"},
			wantMessage: "invalid --output value",
		},
		{
			name: "unreachable server",
			args: []string{
				"--server=http://127.0.0.1:1",
				"--timeout=50ms",
				"--output=json",
				"status",
			},
			wantMessage: "request failed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var (
				stdout bytes.Buffer
				stderr bytes.Buffer
			)
			code := Run(Invocation{
				Args:   test.args,
				Stdout: &stdout,
				Stderr: &stderr,
			}, testDependencies(&stubFileSystem{}))
			if code != 1 {
				t.Fatalf("exit code = %d, want 1", code)
			}

			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}

			if !strings.Contains(stderr.String(), test.wantMessage) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), test.wantMessage)
			}
		})
	}
}

func TestRunUsesInjectedEnvironmentForAuthentication(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		username, password, ok := request.BasicAuth()
		if !ok || username != "admin" || password != "test-password" {
			t.Errorf("basic auth = (%q, %q, %t)", username, password, ok)
		}

		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/readyz" {
			_, _ = writer.Write([]byte(`{"status":"ready","http":true,"torrent":true}`))

			return
		}

		_, _ = writer.Write([]byte(compatibleVersionResponse))
	}))
	t.Cleanup(server.Close)

	dependencies := testDependencies(&stubFileSystem{})
	dependencies.Getenv = func(name string) string {
		switch name {
		case envUser:
			return "admin"
		case envPassword:
			return "test-password"
		default:
			return ""
		}
	}

	var stderr bytes.Buffer
	code := Run(Invocation{
		Args:   []string{"--server", server.URL, "status"},
		Stdout: io.Discard,
		Stderr: &stderr,
	}, dependencies)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
}

func TestRunUsesInjectedInputForDestructiveConfirmation(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	dependencies := testDependencies(&stubFileSystem{})
	dependencies.IsTerminal = func(io.Reader) bool { return true }

	var stderr bytes.Buffer
	code := Run(Invocation{
		Args:   []string{"--server", server.URL, "torrents", "wipe"},
		Stdin:  strings.NewReader("yes\n"),
		Stdout: io.Discard,
		Stderr: &stderr,
	}, dependencies)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}

	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}

	if !strings.Contains(stderr.String(), "Type 'yes' to continue") {
		t.Fatalf("confirmation prompt missing: %q", stderr.String())
	}
}

func testDependencies(fileSystem FileSystem) Dependencies {
	dependencies := DefaultDependencies()
	dependencies.Getenv = func(string) string { return "" }
	dependencies.FileSystem = fileSystem
	dependencies.IsTerminal = func(io.Reader) bool { return false }

	return dependencies
}

type stubFileSystem struct {
	failOnAccess bool
}

func (fileSystem *stubFileSystem) ReadFile(string) ([]byte, error) {
	fileSystem.checkAccess()

	return nil, fs.ErrNotExist
}

func (fileSystem *stubFileSystem) WriteFile(string, []byte, fs.FileMode) error {
	fileSystem.checkAccess()

	return nil
}

func (fileSystem *stubFileSystem) MkdirAll(string, fs.FileMode) error {
	fileSystem.checkAccess()

	return nil
}

func (fileSystem *stubFileSystem) Stat(string) (fs.FileInfo, error) {
	fileSystem.checkAccess()

	return nil, fs.ErrNotExist
}

func (fileSystem *stubFileSystem) UserConfigDir() (string, error) {
	fileSystem.checkAccess()

	return "/virtual/config", nil
}

func (fileSystem *stubFileSystem) checkAccess() {
	if fileSystem.failOnAccess {
		panic("unexpected filesystem access")
	}
}
