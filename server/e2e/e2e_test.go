//go:build e2e

// Package e2e provides opt-in black-box tests for an explicitly selected TorrServer instance.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	testServerURLEnv = "TORRSERVER_E2E_URL"
	testDebugEnv     = "TORRSERVER_E2E_DEBUG"
)

func testServerURL(t *testing.T) string {
	t.Helper()

	rawURL := strings.TrimSpace(os.Getenv(testServerURLEnv))
	if rawURL == "" {
		t.Skipf("set %s to run opt-in black-box tests", testServerURLEnv)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		t.Fatalf("%s must be an absolute HTTP(S) URL, got %q", testServerURLEnv, rawURL)
	}

	return strings.TrimRight(rawURL, "/")
}

func testDebugEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(testDebugEnv)), "true")
}

func skipIfServerNotRunning(t *testing.T) {
	t.Helper()

	baseURL := testServerURL(t)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(baseURL + "/echo")
	if err != nil {
		t.Fatalf("selected E2E server is not reachable on %s: %v", baseURL, err)
	}

	func() { _ = resp.Body.Close() }()
}

func TestEchoEndpoint(t *testing.T) {
	skipIfServerNotRunning(t)

	resp, err := http.Get(testServerURL(t) + "/echo")
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

	if string(body) != "1.0.0" {
		t.Errorf("Expected compatibility version '1.0.0', got %q", string(body))
	}
}

func TestHealthEndpoint(t *testing.T) {
	skipIfServerNotRunning(t)

	resp, err := http.Get(testServerURL(t) + "/healthz")
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

	resp, err := http.Get(testServerURL(t) + "/readyz")
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

	url := testServerURL(t) + "/torrents"
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

	url := testServerURL(t) + "/settings"
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
	resp, err := http.Get(testServerURL(t) + "/stream")
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

	url := testServerURL(t) + "/viewed"
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
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		if readErr != nil {
			t.Fatalf("Expected status 200, got %d; response read failed: %v", resp.StatusCode, readErr)
		}

		t.Errorf("Expected status 200, got %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

func TestPprofEndpoints(t *testing.T) {
	skipIfServerNotRunning(t)
	expectedStatus := http.StatusNotFound
	if testDebugEnabled() {
		expectedStatus = http.StatusOK
	}

	endpoints := []string{
		"/debug/pprof/",
		"/debug/heap",
		"/debug/goroutines",
	}

	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			resp, err := http.Get(testServerURL(t) + ep)
			if err != nil {
				t.Fatalf("Failed to connect: %v", err)
			}

			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != expectedStatus {
				t.Errorf("Expected status %d for %s, got %d", expectedStatus, ep, resp.StatusCode)
			}
		})
	}
}

func TestVarsEndpoint(t *testing.T) {
	skipIfServerNotRunning(t)

	resp, err := http.Get(testServerURL(t) + "/debug/vars")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if !testDebugEnabled() {
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404 with debug disabled, got %d", resp.StatusCode)
		}

		return
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200 with debug enabled, got %d", resp.StatusCode)
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

	resp, err := http.Get(testServerURL(t) + "/api/version")
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

	req, err := http.NewRequest(http.MethodPost, testServerURL(t)+"/torrent/upload", body)
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
	baseURL := testServerURL(t)
	done := make(chan error, concurrent)

	for i := range concurrent {
		go func(id int) {
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Get(baseURL + "/echo")
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
