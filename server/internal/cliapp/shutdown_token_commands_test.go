package cliapp

import (
	"bytes"
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

func TestShutdownTokenCLIStatusGenerateAndSet(t *testing.T) {
	configureTestContextPath(t)
	t.Setenv(envUser, "admin")
	t.Setenv(envPassword, "test-password")

	const generatedToken = "generated-shutdown-token-1234567890"

	var storedToken string

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, pass, ok := request.BasicAuth()
		if !ok || user != "admin" || pass != "test-password" {
			t.Errorf("basic auth = (%q, %q, %t)", user, pass, ok)
		}

		writer.Header().Set("Content-Type", "application/json")

		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/config/shutdown-token":
			writeShutdownTokenTestJSON(t, writer, map[string]any{"configured": storedToken != ""})
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/config/shutdown-token/generate":
			storedToken = generatedToken
			writeShutdownTokenTestJSON(t, writer, map[string]any{"status": "generated", "token": generatedToken})
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/config/shutdown-token":
			var payload map[string]string
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Errorf("decode set token request: %v", err)

				return
			}

			storedToken = payload["token"]

			writeShutdownTokenTestJSON(t, writer, map[string]any{"status": "ok"})
		default:
			t.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	status := executeShutdownTokenCLI(t, server.URL, outputJSON, "config", "shutdown-token", "status")
	if status.code != 0 || !strings.Contains(status.stdout, `"configured": false`) {
		t.Fatalf("initial status: code=%d stdout=%s stderr=%s", status.code, status.stdout, status.stderr)
	}

	generated := executeShutdownTokenCLI(t, server.URL, outputTable, "config", "shutdown-token", "generate", "--yes")
	if generated.code != 0 || generated.stdout != generatedToken+"\n" {
		t.Fatalf("generate: code=%d stdout=%q stderr=%q", generated.code, generated.stdout, generated.stderr)
	}

	if strings.Contains(generated.stderr, generatedToken) {
		t.Fatalf("generated token leaked to stderr: %s", generated.stderr)
	}

	generatedJSON := executeShutdownTokenCLI(t, server.URL, outputJSON, "config", "shutdown-token", "generate", "--yes")
	if generatedJSON.code != 0 || strings.Count(generatedJSON.stdout, generatedToken) != 1 {
		t.Fatalf(
			"JSON generate: code=%d stdout=%q stderr=%q",
			generatedJSON.code,
			generatedJSON.stdout,
			generatedJSON.stderr,
		)
	}

	if strings.Contains(generatedJSON.stderr, generatedToken) {
		t.Fatalf("generated token leaked to JSON stderr: %s", generatedJSON.stderr)
	}

	const explicitToken = "explicit-shutdown-token-1234567890"

	t.Setenv(envToken, explicitToken)

	set := executeShutdownTokenCLI(t, server.URL, outputJSON, "config", "shutdown-token", "set", "--yes")
	if set.code != 0 || storedToken != explicitToken {
		t.Fatalf("set: code=%d stored=%q stdout=%s stderr=%s", set.code, storedToken, set.stdout, set.stderr)
	}

	if strings.Contains(set.stdout, explicitToken) || strings.Contains(set.stderr, explicitToken) {
		t.Fatal("explicit shutdown token was printed")
	}

	configured := executeShutdownTokenCLI(t, server.URL, outputJSON, "config", "shutdown-token", "status")
	if configured.code != 0 || !strings.Contains(configured.stdout, `"configured": true`) {
		t.Fatalf("configured status: code=%d stdout=%s stderr=%s", configured.code, configured.stdout, configured.stderr)
	}
}

func TestShutdownTokenRotationRequiresConfirmation(t *testing.T) {
	configureTestContextPath(t)

	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	t.Cleanup(server.Close)

	for _, operation := range []string{"generate", "set"} {
		t.Run(operation, func(t *testing.T) {
			command := newConfigCmd(&globalOptions{
				Server:  server.URL,
				Token:   "confirmation-test-token",
				Timeout: time.Second,
				Output:  outputTable,
			})
			command.SetArgs([]string{"shutdown-token", operation})
			command.SetIn(strings.NewReader(""))
			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})

			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), "--yes") {
				t.Fatalf("confirmation error = %v", err)
			}

			if requests.Load() != 0 {
				t.Fatalf("requests = %d, want 0", requests.Load())
			}
		})
	}
}

func TestShutdownTokenCLIAuthErrorRedactsSecret(t *testing.T) {
	configureTestContextPath(t)
	t.Setenv(envUser, "admin")
	t.Setenv(envPassword, "wrong-password")

	const secret = "do-not-print-shutdown-token" // #nosec G101 -- redaction test fixture, not a credential.

	t.Setenv(envToken, secret)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnauthorized)
		writeShutdownTokenTestJSON(t, writer, map[string]any{
			"error": map[string]any{
				"type":    "unauthorized",
				"message": "invalid token=" + secret,
			},
		})
	}))
	t.Cleanup(server.Close)

	result := executeShutdownTokenCLI(t, server.URL, outputJSON, "config", "shutdown-token", "set", "--yes")
	if result.code == 0 || result.stdout != "" {
		t.Fatalf("auth failure: code=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}

	if strings.Contains(result.stderr, secret) || !strings.Contains(result.stderr, "[redacted]") {
		t.Fatalf("secret was not redacted: %s", result.stderr)
	}
}

func TestShutdownTokenStatusHonorsCommandCancellation(t *testing.T) {
	t.Parallel()

	requestStarted := make(chan struct{})
	requestCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		<-request.Context().Done()
		close(requestCanceled)
	}))
	t.Cleanup(server.Close)

	command := newConfigCmd(&globalOptions{Server: server.URL, Timeout: time.Minute, Output: outputTable})
	command.SetArgs([]string{"shutdown-token", "status"})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})

	ctx, cancel := context.WithCancel(context.Background())
	command.SetContext(ctx)

	result := make(chan error, 1)

	go func() {
		result <- command.Execute()
	}()

	<-requestStarted
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("status command did not stop after cancellation")
	}

	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("status HTTP request was not canceled")
	}
}

type shutdownTokenCLIResult struct {
	code   int
	stdout string
	stderr string
}

func executeShutdownTokenCLI(
	t *testing.T,
	serverURL string,
	output string,
	args ...string,
) shutdownTokenCLIResult {
	t.Helper()

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	global := []string{"--server", serverURL, "--output", output}
	code := executeCLI(append(global, args...), &stdout, &stderr)

	return shutdownTokenCLIResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func writeShutdownTokenTestJSON(t *testing.T, writer http.ResponseWriter, payload any) {
	t.Helper()

	if err := json.NewEncoder(writer).Encode(payload); err != nil {
		t.Errorf("encode shutdown-token response: %v", err)
	}
}
