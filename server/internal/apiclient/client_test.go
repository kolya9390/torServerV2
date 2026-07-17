package apiclient

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

func TestNormalizeBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{name: "default", want: DefaultBaseURL + "/"},
		{name: "implicit HTTP", value: "localhost:8090", want: "http://localhost:8090/"},
		{name: "HTTPS base path", value: "https://example.test/proxy/torrserver", want: "https://example.test/proxy/torrserver/"},
		{name: "missing host", value: "http:///missing-host", wantErr: true},
		{name: "unsupported scheme", value: "ftp://example.test", wantErr: true},
		{name: "URL credentials", value: "http://user:password@example.test", wantErr: true},
		{name: "query", value: "http://example.test/base?token=secret", wantErr: true},
		{name: "fragment", value: "http://example.test/base#fragment", wantErr: true},
		{name: "invalid IPv6", value: "http://[::1", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := normalizeBaseURL(test.value)
			if test.wantErr {
				if err == nil {
					t.Fatalf("normalizeBaseURL(%q) unexpectedly succeeded", test.value)
				}

				return
			}

			if err != nil {
				t.Fatalf("normalizeBaseURL(%q): %v", test.value, err)
			}

			if got := parsed.String(); got != test.want {
				t.Fatalf("URL = %q, want %q", got, test.want)
			}
		})
	}
}

func TestClientPreservesConfiguredBasePath(t *testing.T) {
	t.Parallel()

	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestedPath = request.URL.Path
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, compatibleVersionResponse)
	}))
	t.Cleanup(server.Close)

	client := mustNewClient(t, Options{BaseURL: server.URL + "/proxy/torrserver"})
	if _, err := client.Version(context.Background()); err != nil {
		t.Fatalf("Version through base path: %v", err)
	}

	if requestedPath != "/proxy/torrserver/api/v1/version" {
		t.Fatalf("requested path = %q", requestedPath)
	}
}

func TestClientRejectsRedirects(t *testing.T) {
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

	client := mustNewClient(t, Options{BaseURL: server.URL})
	_, err := client.Version(context.Background())
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "redirect rejected") {
		t.Fatalf("redirect error = %v", err)
	}

	if redirected.Load() {
		t.Fatal("redirect target was requested")
	}
}

func TestClientBoundsSuccessAndErrorResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		configure  func(*Client)
	}{
		{
			name:       "success",
			statusCode: http.StatusOK,
			configure: func(client *Client) {
				client.maxResponse = 8
			},
		},
		{
			name:       "error",
			statusCode: http.StatusBadRequest,
			configure: func(client *Client) {
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

			client := mustNewClient(t, Options{BaseURL: server.URL})
			test.configure(client)

			err := client.doJSON(context.Background(), http.MethodGet, "/api/test", nil, nil, nil)

			var limitErr *ResponseLimitError
			if !errors.As(err, &limitErr) {
				t.Fatalf("error = %v, want ResponseLimitError", err)
			}

			if limitErr.Limit != 8 || !strings.Contains(err.Error(), "8-byte CLI limit") {
				t.Fatalf("unexpected limit error: %v", err)
			}
		})
	}
}

func TestClientDecodesStructuredAndLegacyErrorsSafely(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "structured",
			body: `{"error":{"type":"validation_error","message":"password=secret Bearer abc http://user:pass@example.test","field":"token"},"request_id":"request-1"}`,
		},
		{
			name: "legacy string",
			body: `{"error":"password=secret Bearer abc http://user:pass@example.test"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(writer, test.body)
			}))
			t.Cleanup(server.Close)

			client := mustNewClient(t, Options{BaseURL: server.URL})
			_, err := client.Version(context.Background())

			var responseErr *ResponseError
			if !errors.As(err, &responseErr) {
				t.Fatalf("error = %v, want ResponseError", err)
			}

			combined := responseErr.Message + " " + responseErr.Error()
			for _, secret := range []string{"secret", "Bearer abc", "user:pass"} {
				if strings.Contains(combined, secret) {
					t.Fatalf("error disclosed %q: %s", secret, combined)
				}
			}
		})
	}
}

func TestClientRejectsMalformedSuccessJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"current":`)
	}))
	t.Cleanup(server.Close)

	client := mustNewClient(t, Options{BaseURL: server.URL})
	_, err := client.Version(context.Background())
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Fatalf("malformed JSON error = %v", err)
	}
}

func TestClientCancellationStopsRequest(t *testing.T) {
	t.Parallel()

	requestStarted := make(chan struct{})
	requestDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		<-request.Context().Done()
		close(requestDone)
	}))
	t.Cleanup(server.Close)

	client := mustNewClient(t, Options{BaseURL: server.URL, Timeout: time.Minute})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)

	go func() {
		_, err := client.Version(ctx)
		result <- err
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

func TestClientTLSRequiresExplicitInsecureOptIn(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, compatibleVersionResponse)
	}))
	t.Cleanup(server.Close)

	secureClient := mustNewClient(t, Options{BaseURL: server.URL})
	if _, err := secureClient.Version(context.Background()); err == nil {
		t.Fatal("self-signed TLS certificate unexpectedly trusted")
	}

	insecureClient := mustNewClient(t, Options{BaseURL: server.URL, Insecure: true})
	if _, err := insecureClient.Version(context.Background()); err != nil {
		t.Fatalf("explicit insecure TLS request: %v", err)
	}
}

func TestClientReportsResponseCloseFailure(t *testing.T) {
	t.Parallel()

	closeFailure := errors.New("close failed")
	body := &observableCloseBody{Reader: strings.NewReader(`{}`), closeErr: closeFailure}
	baseURL, err := normalizeBaseURL("http://example.test")
	if err != nil {
		t.Fatalf("normalize base URL: %v", err)
	}

	client := &Client{
		baseURL: baseURL,
		http: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
		})},
		maxResponse:      defaultMaxResponseBytes,
		maxErrorResponse: defaultMaxErrorResponseBytes,
	}

	err = client.doJSON(context.Background(), http.MethodGet, "/api/test", nil, nil, nil)
	if !errors.Is(err, closeFailure) {
		t.Fatalf("close error = %v", err)
	}

	if !body.closed.Load() {
		t.Fatal("response body was not closed")
	}
}

func TestUploadTorrentReportsMultipartReadFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{}`)
	}))
	t.Cleanup(server.Close)

	client := mustNewClient(t, Options{BaseURL: server.URL})
	_, err := client.UploadTorrent(context.Background(), UploadTorrentRequest{FilePath: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "copy torrent file") {
		t.Fatalf("multipart failure = %v", err)
	}
}

func mustNewClient(t *testing.T, options Options) *Client {
	t.Helper()

	client, err := New(options)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	t.Cleanup(client.CloseIdleConnections)

	return client
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
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
