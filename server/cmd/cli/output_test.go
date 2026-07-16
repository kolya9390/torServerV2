package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

const outputContractTorrentHash = "0123456789abcdef0123456789abcdef01234567"

func TestExecuteCLI_JSONSuccessContractAcrossCommandFamilies(t *testing.T) {
	t.Setenv(envConfig, filepath.Join(t.TempDir(), "contexts.json"))

	server := httptest.NewServer(http.HandlerFunc(jsonContractHandler))
	t.Cleanup(server.Close)

	tests := []struct {
		name string
		args []string
	}{
		{name: "context", args: []string{"--output", "json", "context", "current"}},
		{name: "status", args: []string{"--server", server.URL, "--output", "json", "status"}},
		{name: "torrent", args: []string{"--server", server.URL, "--output", "json", "torrents", "list"}},
		{
			name: "url",
			args: []string{
				"--server", server.URL,
				"--output", "json",
				"url", outputContractTorrentHash,
				"--file", "1",
			},
		},
		{name: "settings", args: []string{"--server", server.URL, "--output", "json", "settings", "get"}},
		{name: "auth", args: []string{"--server", server.URL, "--output", "json", "auth", "list"}},
		{name: "shutdown write", args: []string{"--server", server.URL, "--output", "json", "shutdown"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				stdout bytes.Buffer
				stderr bytes.Buffer
			)

			if code := executeCLI(test.args, &stdout, &stderr); code != 0 {
				t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
			}

			if stderr.Len() != 0 {
				t.Fatalf("stderr is not empty: %s", stderr.String())
			}

			var envelope struct {
				OK   bool            `json:"ok"`
				Data json.RawMessage `json:"data"`
			}

			decodeSingleJSONValue(t, stdout.Bytes(), &envelope)

			if !envelope.OK {
				t.Fatal("success envelope has ok=false")
			}

			if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
				t.Fatal("success envelope has empty data")
			}

			if strings.Contains(stdout.String(), "OK:") || strings.Contains(stdout.String(), "Next:") {
				t.Fatalf("JSON stdout contains human text: %s", stdout.String())
			}
		})
	}
}

func TestExecuteCLI_JSONValidationErrorUsesStderrOnly(t *testing.T) {
	t.Setenv(envConfig, filepath.Join(t.TempDir(), "contexts.json"))

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	code := executeCLI([]string{"--output=json", "settings", "set"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}

	if stdout.Len() != 0 {
		t.Fatalf("stdout is not empty: %s", stdout.String())
	}

	var envelope errorEnvelope

	decodeSingleJSONValue(t, stderr.Bytes(), &envelope)

	if envelope.OK {
		t.Fatal("error envelope has ok=true")
	}

	if envelope.Error.Code != "command_error" {
		t.Fatalf("error code = %q, want command_error", envelope.Error.Code)
	}

	if !strings.Contains(envelope.Error.Message, "requires at least 1 arg") {
		t.Fatalf("unexpected validation message %q", envelope.Error.Message)
	}
}

func TestExecuteCLI_JSONAPIErrorIsStructuredAndSanitized(t *testing.T) {
	t.Setenv(envConfig, filepath.Join(t.TempDir(), "contexts.json"))

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = writer.Write([]byte(`{
  "error": {
    "type": "validation_error",
    "message": "invalid token=super-secret\nvalue",
    "field": "link"
  },
  "request_id": "request-123"
}`))
	}))
	t.Cleanup(server.Close)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	code := executeCLI(
		[]string{"--server", server.URL, "--output", "json", "torrents", "list"},
		&stdout,
		&stderr,
	)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}

	if stdout.Len() != 0 {
		t.Fatalf("stdout is not empty: %s", stdout.String())
	}

	var envelope errorEnvelope

	decodeSingleJSONValue(t, stderr.Bytes(), &envelope)

	if envelope.Error.Code != "validation_error" {
		t.Fatalf("error code = %q", envelope.Error.Code)
	}

	if envelope.Error.Status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", envelope.Error.Status)
	}

	if envelope.Error.Field != "link" || envelope.Error.RequestID != "request-123" {
		t.Fatalf("unexpected API error metadata: %+v", envelope.Error)
	}

	if strings.Contains(stderr.String(), "super-secret") || !strings.Contains(envelope.Error.Message, "[redacted]") {
		t.Fatalf("API error was not sanitized: %s", stderr.String())
	}
}

func TestErrorPayloadBoundsAndRedactsGenericErrors(t *testing.T) {
	secret := "do-not-print"
	longSuffix := strings.Repeat("x", maxCLIErrorRunes*2)
	err := errors.New("request http://user:" + secret + "@example.test?token=" + secret + " " + longSuffix)

	payload := errorPayload(err)
	if strings.Contains(payload.Message, secret) {
		t.Fatalf("secret leaked in error message: %s", payload.Message)
	}

	if len([]rune(payload.Message)) > maxCLIErrorRunes+3 {
		t.Fatalf("message has %d runes", len([]rune(payload.Message)))
	}
}

func TestRedactURLCredentials(t *testing.T) {
	secret := "do-not-print"
	redacted := redactURLCredentials(
		"https://user:" + secret + "@example.test/base?token=" + secret + "&mode=read",
	)
	if strings.Contains(redacted, secret) || strings.Contains(redacted, "user") {
		t.Fatalf("credentials leaked in URL: %s", redacted)
	}

	if !strings.Contains(redacted, "mode=read") || !strings.Contains(redacted, "%5Bredacted%5D") {
		t.Fatalf("unexpected redacted URL: %s", redacted)
	}
}

func TestRequestedJSONOutput(t *testing.T) {
	for _, args := range [][]string{
		{"--output=json", "status"},
		{"status", "--output", "JSON"},
	} {
		if !requestedJSONOutput(args) {
			t.Fatalf("JSON output not detected in %v", args)
		}
	}
}

func jsonContractHandler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")

	switch request.URL.Path {
	case "/api/v1/version":
		_, _ = writer.Write([]byte(`{"current":"v1.0.0-beta.1"}`))
	case "/readyz":
		_, _ = writer.Write([]byte(`{"status":"ready","http":true,"torrent":true}`))
	case "/api/v1/torrents":
		_, _ = writer.Write([]byte(`[{"hash":"` + outputContractTorrentHash + `","title":"Movie"}]`))
	case "/api/v1/settings":
		_, _ = writer.Write([]byte(`{"CacheSize":67108864}`))
	case "/api/v1/auth/users":
		_, _ = writer.Write([]byte(`{"admin":"2026-07-14T00:00:00Z"}`))
	case "/api/v1/shutdown/torrctl":
		writer.WriteHeader(http.StatusAccepted)
	default:
		http.NotFound(writer, request)
	}
}

func decodeSingleJSONValue(t *testing.T, data []byte, target any) {
	t.Helper()

	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decode JSON: %v; payload: %s", err, data)
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		t.Fatalf("multiple JSON values in payload: %s", data)
	}
}
