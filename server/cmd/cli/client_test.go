package cli

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNormalizeServerURL(t *testing.T) {
	t.Parallel()

	valid := []struct {
		name     string
		value    string
		expected string
	}{
		{name: "default", expected: defaultServerURL + "/"},
		{name: "implicit HTTP", value: "localhost:8090", expected: "http://localhost:8090/"},
		{
			name:     "HTTPS base path",
			value:    "https://example.test/proxy/torrserver",
			expected: "https://example.test/proxy/torrserver/",
		},
	}

	for _, test := range valid {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := normalizeServerURL(test.value)
			if err != nil {
				t.Fatalf("normalize server URL: %v", err)
			}

			if parsed.String() != test.expected {
				t.Fatalf("URL = %q, want %q", parsed.String(), test.expected)
			}
		})
	}

	invalid := []string{
		"http:///missing-host",
		"ftp://example.test",
		"http://user:password@example.test",
		"http://example.test/base?token=secret",
		"http://example.test/base#fragment",
		"http://[::1",
	}
	for _, value := range invalid {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			if _, err := normalizeServerURL(value); err == nil {
				t.Fatalf("invalid server URL %q was accepted", value)
			}
		})
	}
}

func TestAPIClientPreservesConfiguredBasePath(t *testing.T) {
	t.Parallel()

	var requestedPath string

	var response map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestedPath = request.URL.Path

		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	client := mustNewAPIClient(t, server.URL+"/proxy/torrserver")

	if err := client.doJSON(context.Background(), http.MethodGet, "/api/v1/version", nil, &response, nil); err != nil {
		t.Fatalf("request through base path: %v", err)
	}

	if requestedPath != "/proxy/torrserver/api/v1/version" {
		t.Fatalf("requested path = %q", requestedPath)
	}
}

func TestAPIClientRejectsRedirects(t *testing.T) {
	t.Parallel()

	var redirected atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/final" {
			redirected.Store(true)

			_, _ = io.WriteString(writer, `{}`)

			return
		}

		http.Redirect(writer, request, "/final", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(server.Close)

	client := mustNewAPIClient(t, server.URL)
	err := client.doJSON(context.Background(), http.MethodGet, "/api/v1/version", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "redirect rejected") {
		t.Fatalf("redirect error = %v", err)
	}

	if redirected.Load() {
		t.Fatal("redirect target was requested")
	}
}

func TestAPIClientBoundsSuccessAndErrorResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		configure  func(*apiClient)
	}{
		{
			name:       "success",
			statusCode: http.StatusOK,
			configure: func(client *apiClient) {
				client.maxResponse = 8
			},
		},
		{
			name:       "error",
			statusCode: http.StatusBadRequest,
			configure: func(client *apiClient) {
				client.maxErrorResponse = 8
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.statusCode)
				_, _ = io.WriteString(writer, strings.Repeat("x", 9))
			}))
			t.Cleanup(server.Close)

			client := mustNewAPIClient(t, server.URL)
			test.configure(client)

			err := client.doJSON(context.Background(), http.MethodGet, "/api/test", nil, nil, nil)

			var limitErr *responseLimitError

			if !errors.As(err, &limitErr) {
				t.Fatalf("error = %v, want responseLimitError", err)
			}

			if limitErr.Limit != 8 || !strings.Contains(err.Error(), "8-byte CLI limit") {
				t.Fatalf("unexpected limit error: %v", err)
			}
		})
	}
}

func TestAPIClientCancellation(t *testing.T) {
	t.Parallel()

	requestStarted := make(chan struct{})
	requestDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		<-request.Context().Done()
		close(requestDone)
	}))
	t.Cleanup(server.Close)

	client := mustNewAPIClient(t, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)

	go func() {
		result <- client.doJSON(ctx, http.MethodGet, "/api/test", nil, nil, nil)
	}()

	<-requestStarted
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("client request did not stop after cancellation")
	}

	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("server request context was not canceled")
	}
}

func TestAPIClientTLSRequiresExplicitInsecureOptIn(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{}`)
	}))
	t.Cleanup(server.Close)

	secureClient := mustNewAPIClientWithOptions(t, globalOptions{Server: server.URL, Timeout: time.Second})
	if err := secureClient.doJSON(context.Background(), http.MethodGet, "/api/test", nil, nil, nil); err == nil {
		t.Fatal("self-signed TLS certificate unexpectedly trusted")
	}

	insecureClient := mustNewAPIClientWithOptions(t, globalOptions{
		Server:   server.URL,
		Timeout:  time.Second,
		Insecure: true,
	})
	if err := insecureClient.doJSON(context.Background(), http.MethodGet, "/api/test", nil, nil, nil); err != nil {
		t.Fatalf("explicit insecure TLS request: %v", err)
	}
}

func TestAPIClientReportsResponseCloseFailure(t *testing.T) {
	t.Parallel()

	closeFailure := errors.New("close failed")
	body := &observableCloseBody{
		Reader:   strings.NewReader(`{}`),
		closeErr: closeFailure,
	}
	baseURL, err := normalizeServerURL("http://example.test")
	if err != nil {
		t.Fatalf("normalize base URL: %v", err)
	}

	client := &apiClient{
		baseURL: baseURL,
		http: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
		})},
		maxResponse:      defaultMaxCLIResponseBytes,
		maxErrorResponse: defaultMaxCLIErrorResponseBytes,
	}

	err = client.doJSON(context.Background(), http.MethodGet, "/api/test", nil, nil, nil)
	if !errors.Is(err, closeFailure) {
		t.Fatalf("close error = %v", err)
	}

	if !body.closed.Load() {
		t.Fatal("response body was not closed")
	}
}

func mustNewAPIClient(t *testing.T, serverURL string) *apiClient {
	t.Helper()

	return mustNewAPIClientWithOptions(t, globalOptions{Server: serverURL, Timeout: time.Second})
}

func mustNewAPIClientWithOptions(t *testing.T, opts globalOptions) *apiClient {
	t.Helper()

	client, err := newAPIClient(opts)
	if err != nil {
		t.Fatalf("new API client: %v", err)
	}

	t.Cleanup(client.http.CloseIdleConnections)

	return client
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type observableCloseBody struct {
	io.Reader
	closeErr error
	closed   atomic.Bool
}

func (body *observableCloseBody) Close() error {
	body.closed.Store(true)

	return body.closeErr
}
