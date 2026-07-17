package cliapp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

const compatibleVersionResponse = `{"product":"torrserver","application_version":"v1.0.0-beta.3",` +
	`"current":"v1","capabilities":["management-api-v1"]}`

func TestStatusReturnsStructuredCompatibilityFailureWithoutReadinessRequest(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Path != "/api/v1/version" {
			t.Errorf("unexpected request path %q", request.URL.Path)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"product":"torrserver","application_version":"v2.0.0",` +
			`"current":"v2","capabilities":["management-api-v2"]}`))
	}))
	t.Cleanup(server.Close)

	var stderr bytes.Buffer
	code := executeCLI(
		[]string{"--server", server.URL, "--output=json", "status"},
		&bytes.Buffer{},
		&stderr,
	)
	if code != 1 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}

	var envelope errorEnvelope
	decodeSingleJSONValue(t, stderr.Bytes(), &envelope)
	if envelope.Error.Code != "unsupported_api_version" {
		t.Fatalf("error = %+v", envelope.Error)
	}

	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want only the version request", requests.Load())
	}
}

func TestTorrentCommandDoesNotPerformCompatibilityHandshake(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Path != "/api/v1/torrents" {
			t.Errorf("unexpected request path %q", request.URL.Path)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[]`))
	}))
	t.Cleanup(server.Close)

	var stdout bytes.Buffer
	code := executeCLI(
		[]string{"--server", server.URL, "--output=json", "torrents", "list"},
		&stdout,
		&bytes.Buffer{},
	)
	if code != 0 {
		t.Fatalf("exit code = %d, stdout = %s", code, stdout.String())
	}

	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want one command request", requests.Load())
	}

	var envelope successEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK {
		t.Fatalf("response = %s, error = %v", stdout.String(), err)
	}
}
