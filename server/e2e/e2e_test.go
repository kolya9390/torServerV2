// Package e2e provides end-to-end integration tests for the TorrServer API.
// Tests connect to a running server instance on localhost:8090.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

const testServerURL = "http://127.0.0.1:8090"

func skipIfServerNotRunning(t *testing.T) {
	t.Helper()

	resp, err := http.Get(testServerURL + "/echo")
	if err != nil {
		t.Skipf("Server not running on %s: %v", testServerURL, err)
	}

	func() { _ = resp.Body.Close() }()
}

func TestEchoEndpoint(t *testing.T) {
	skipIfServerNotRunning(t)

	resp, err := http.Get(testServerURL + "/echo")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if string(body) != "2.0.0" {
		t.Errorf("Expected '2.0.0', got %q", string(body))
	}
}

func TestHealthEndpoint(t *testing.T) {
	skipIfServerNotRunning(t)

	resp, err := http.Get(testServerURL + "/healthz")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if string(body) != "OK" {
		t.Errorf("Expected 'OK', got %q", string(body))
	}
}

func TestReadyzEndpoint(t *testing.T) {
	skipIfServerNotRunning(t)

	resp, err := http.Get(testServerURL + "/readyz")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var status map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if status["status"] != "ready" {
		t.Errorf("Expected status 'ready', got %v", status["status"])
	}
}

func TestListTorrents(t *testing.T) {
	skipIfServerNotRunning(t)

	url := testServerURL + "/torrents"
	payload := `{"action":"list"}`

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestSettingsGet(t *testing.T) {
	skipIfServerNotRunning(t)

	url := testServerURL + "/settings"
	payload := `{"action":"get"}`

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var settings map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		t.Fatalf("Failed to decode settings: %v", err)
	}

	// Verify key settings are present
	if _, ok := settings["CacheSize"]; !ok {
		t.Error("CacheSize not found in settings response")
	}
}

func TestStreamEndpointExists(t *testing.T) {
	skipIfServerNotRunning(t)

	// Test that /stream endpoint exists (will return 400 without proper params)
	resp, err := http.Get(testServerURL + "/stream")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	// Without valid link param, should return 400
	if resp.StatusCode != http.StatusBadRequest {
		t.Logf("Stream endpoint returned status %d (400 expected without params)", resp.StatusCode)
	}
}

func TestViewedList(t *testing.T) {
	skipIfServerNotRunning(t)

	url := testServerURL + "/viewed"
	payload := `{"action":"list"}`

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	// Endpoint should exist, may return empty list
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestPprofEndpoints(t *testing.T) {
	skipIfServerNotRunning(t)

	endpoints := []string{
		"/debug/pprof/",
		"/debug/heap",
		"/debug/goroutines",
	}

	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			resp, err := http.Get(testServerURL + ep)
			if err != nil {
				t.Fatalf("Failed to connect: %v", err)
			}

			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200 for %s, got %d", ep, resp.StatusCode)
			}
		})
	}
}

func TestVarsEndpoint(t *testing.T) {
	skipIfServerNotRunning(t)

	resp, err := http.Get(testServerURL + "/debug/vars")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var vars map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&vars); err != nil {
		t.Fatalf("Failed to decode vars: %v", err)
	}

	// Verify expected metrics exist
	expectedKeys := []string{"goroutines", "heap_alloc_bytes", "memstats"}
	for _, key := range expectedKeys {
		if _, ok := vars[key]; !ok {
			t.Errorf("Expected key %q not found in /debug/vars", key)
		}
	}
}

func TestAPIVersionEndpoint(t *testing.T) {
	skipIfServerNotRunning(t)

	resp, err := http.Get(testServerURL + "/api/version")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestTorrentUploadEndpointExists(t *testing.T) {
	skipIfServerNotRunning(t)

	// Test that upload endpoint exists (will return error without proper file)
	body := bytes.NewBufferString("not-a-torrent")

	req, err := http.NewRequest(http.MethodPost, testServerURL+"/torrent/upload", body)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	// Should return some status (400 or 500 for invalid data)
	if resp.StatusCode < 400 {
		t.Logf("Upload endpoint returned status %d", resp.StatusCode)
	}
}

func TestConcurrentRequests(t *testing.T) {
	skipIfServerNotRunning(t)

	// Test that server handles concurrent requests
	const concurrent = 10
	done := make(chan error, concurrent)

	for i := range concurrent {
		go func(id int) {
			resp, err := http.Get(testServerURL + "/echo")
			if err != nil {
				done <- fmt.Errorf("request %d failed: %v", id, err)

				return
			}

			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				done <- fmt.Errorf("request %d: expected 200, got %d", id, resp.StatusCode)

				return
			}
			done <- nil
		}(i)
	}

	for range concurrent {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
}

func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()
	os.Exit(code)
}
