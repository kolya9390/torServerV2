package apiclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	DefaultBaseURL               = "http://127.0.0.1:8090"
	defaultTimeout               = 15 * time.Second
	defaultMaxResponseBytes      = int64(16 << 20)
	defaultMaxErrorResponseBytes = int64(64 << 10)
)

// Options configures a management API client.
type Options struct {
	BaseURL  string
	User     string
	Password string //nolint:gosec // Explicit Basic Auth input; Options is never serialized or logged.
	Timeout  time.Duration
	Insecure bool
}

// Client is a bounded HTTP client for the TorrServer management API.
type Client struct {
	baseURL          *url.URL
	http             *http.Client
	user             string
	password         string
	maxResponse      int64
	maxErrorResponse int64
}

// New constructs a client without performing network or filesystem I/O.
func New(options Options) (*Client, error) {
	baseURL, err := normalizeBaseURL(options.BaseURL)
	if err != nil {
		return nil, err
	}

	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("default HTTP transport has an unsupported implementation")
	}

	transport := defaultTransport.Clone()
	transport.TLSClientConfig = &tls.Config{ //nolint:gosec // InsecureSkipVerify is an explicit CLI opt-in.
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: options.Insecure,
	}

	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout:       timeout,
			Transport:     transport,
			CheckRedirect: rejectRedirect,
		},
		user:             options.User,
		password:         options.Password,
		maxResponse:      defaultMaxResponseBytes,
		maxErrorResponse: defaultMaxErrorResponseBytes,
	}, nil
}

// BaseURL returns the normalized server base URL without user information.
func (client *Client) BaseURL() string {
	if client == nil || client.baseURL == nil {
		return ""
	}

	return client.baseURL.String()
}

// CloseIdleConnections closes pooled HTTP connections owned by this client.
func (client *Client) CloseIdleConnections() {
	if client == nil || client.http == nil {
		return
	}

	client.http.CloseIdleConnections()
}

func normalizeBaseURL(value string) (*url.URL, error) {
	rawBase := strings.TrimSpace(value)
	if rawBase == "" {
		rawBase = DefaultBaseURL
	}

	if !strings.Contains(rawBase, "://") {
		rawBase = "http://" + rawBase
	}

	parsed, err := url.Parse(rawBase)
	if err != nil {
		return nil, fmt.Errorf("invalid --server value: %w", err)
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("invalid --server scheme %q: use http or https", parsed.Scheme)
	}

	if parsed.Hostname() == "" {
		return nil, errors.New("invalid --server value: host is required")
	}

	if parsed.User != nil {
		return nil, errors.New("invalid --server value: URL credentials are not allowed; use --user and TS_PASSWORD")
	}

	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("invalid --server value: query and fragment are not allowed")
	}

	cleanPath := path.Clean("/" + strings.TrimPrefix(parsed.Path, "/"))
	if cleanPath == "." || cleanPath == "/" {
		parsed.Path = "/"
	} else {
		parsed.Path = strings.TrimRight(cleanPath, "/") + "/"
	}

	parsed.RawPath = ""

	return parsed, nil
}

func rejectRedirect(_ *http.Request, _ []*http.Request) error {
	return errors.New("HTTP redirect rejected: configure --server with the final endpoint URL")
}

func (client *Client) doJSON(
	ctx context.Context,
	method string,
	endpoint string,
	requestBody any,
	responseBody any,
	headers http.Header,
) error {
	var bodyReader io.Reader

	if requestBody != nil {
		payload, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}

		bodyReader = bytes.NewReader(payload)
	}

	request, err := client.newRequest(ctx, method, endpoint, bodyReader)
	if err != nil {
		return err
	}

	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	request.Header.Set("Accept", "application/json")

	for key, values := range headers {
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				request.Header.Add(key, value)
			}
		}
	}

	// The operator-supplied base URL is constrained to HTTP(S), stripped of credentials,
	// and redirects are rejected by New before a request reaches this boundary.
	response, err := client.http.Do(request) //nolint:gosec // G704: validated remote-management endpoint.
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	data, err := client.readAndCloseResponse(response)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if response.StatusCode >= http.StatusBadRequest {
		return parseResponseError(response.StatusCode, data)
	}

	if responseBody == nil || len(data) == 0 {
		return nil
	}

	if err := json.Unmarshal(data, responseBody); err != nil {
		return &ResponseDecodeError{Err: err}
	}

	return nil
}

func (client *Client) doMultipartFile(
	ctx context.Context,
	endpoint string,
	filePath string,
	fields map[string]string,
	responseBody any,
) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open torrent file: %w", err)
	}

	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	writeErr := make(chan error, 1)

	go writeMultipartFile(file, filepath.Base(filePath), fields, multipartWriter, writer, writeErr)

	request, err := client.newRequest(ctx, http.MethodPost, endpoint, reader)
	if err != nil {
		return errors.Join(err, finishMultipartUpload(reader, err, writeErr))
	}

	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	request.Header.Set("Accept", "application/json")

	// The request URL is built from the same validated base URL used by doJSON.
	response, err := client.http.Do(request) //nolint:gosec // G704: validated remote-management endpoint.
	if err != nil {
		requestErr := fmt.Errorf("request failed: %w", err)

		return errors.Join(requestErr, finishMultipartUpload(reader, requestErr, writeErr))
	}

	data, readErr := client.readAndCloseResponse(response)
	uploadErr := finishMultipartUpload(reader, nil, writeErr)

	if readErr != nil {
		return errors.Join(fmt.Errorf("read response: %w", readErr), uploadErr)
	}

	if response.StatusCode >= http.StatusBadRequest {
		return errors.Join(parseResponseError(response.StatusCode, data), uploadErr)
	}

	if uploadErr != nil {
		return fmt.Errorf("upload torrent file: %w", uploadErr)
	}

	if responseBody == nil || len(data) == 0 {
		return nil
	}

	if err := json.Unmarshal(data, responseBody); err != nil {
		return &ResponseDecodeError{Err: err}
	}

	return nil
}

func writeMultipartFile(
	file *os.File,
	fileName string,
	fields map[string]string,
	multipartWriter *multipart.Writer,
	pipeWriter *io.PipeWriter,
	result chan<- error,
) {
	err := writeMultipartContent(file, fileName, fields, multipartWriter)
	if closeErr := file.Close(); closeErr != nil {
		err = errors.Join(err, fmt.Errorf("close torrent file: %w", closeErr))
	}

	if closeErr := multipartWriter.Close(); closeErr != nil {
		err = errors.Join(err, fmt.Errorf("close multipart writer: %w", closeErr))
	}

	if closeErr := pipeWriter.CloseWithError(err); closeErr != nil && !errors.Is(closeErr, io.ErrClosedPipe) {
		err = errors.Join(err, fmt.Errorf("close upload pipe: %w", closeErr))
	}

	result <- err
}

func writeMultipartContent(file *os.File, fileName string, fields map[string]string, writer *multipart.Writer) error {
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return fmt.Errorf("create torrent file form field: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("copy torrent file: %w", err)
	}

	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		value := fields[key]
		if value == "" {
			continue
		}

		if err := writer.WriteField(key, value); err != nil {
			return fmt.Errorf("write multipart field %s: %w", key, err)
		}
	}

	return nil
}

func (client *Client) newRequest(
	ctx context.Context,
	method string,
	endpoint string,
	body io.Reader,
) (*http.Request, error) {
	fullURL, err := client.resolveEndpoint(endpoint)
	if err != nil {
		return nil, fmt.Errorf("build request url: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, method, fullURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if client.user != "" || client.password != "" {
		request.SetBasicAuth(client.user, client.password)
	}

	return request, nil
}

func (client *Client) resolveEndpoint(endpoint string) (*url.URL, error) {
	if client == nil || client.baseURL == nil {
		return nil, errors.New("API client base URL is not configured")
	}

	relative, err := url.Parse(strings.TrimLeft(strings.TrimSpace(endpoint), "/"))
	if err != nil {
		return nil, err
	}

	if relative.IsAbs() || relative.Host != "" {
		return nil, errors.New("API endpoint must be a relative path")
	}

	if relative.Path == "" {
		return nil, errors.New("API endpoint path is required")
	}

	if relative.Path == ".." || strings.HasPrefix(relative.Path, "../") {
		return nil, errors.New("API endpoint must not escape the configured base path")
	}

	return client.baseURL.ResolveReference(relative), nil
}

func (client *Client) readAndCloseResponse(response *http.Response) ([]byte, error) {
	if response == nil || response.Body == nil {
		return nil, errors.New("HTTP response body is missing")
	}

	limit := client.responseLimit(response.StatusCode)
	data, readErr := readBounded(response.Body, limit)

	if closeErr := response.Body.Close(); closeErr != nil {
		readErr = errors.Join(readErr, fmt.Errorf("close response: %w", closeErr))
	}

	return data, readErr
}

func (client *Client) responseLimit(statusCode int) int64 {
	if statusCode >= http.StatusBadRequest {
		if client.maxErrorResponse > 0 {
			return client.maxErrorResponse
		}

		return defaultMaxErrorResponseBytes
	}

	if client.maxResponse > 0 {
		return client.maxResponse
	}

	return defaultMaxResponseBytes
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}

	if int64(len(data)) > limit {
		return nil, &ResponseLimitError{Limit: limit}
	}

	return data, nil
}

func finishMultipartUpload(reader *io.PipeReader, cause error, result <-chan error) error {
	var closeErr error
	if cause != nil {
		closeErr = reader.CloseWithError(cause)
	} else {
		closeErr = reader.Close()
	}

	if closeErr != nil && !errors.Is(closeErr, io.ErrClosedPipe) {
		closeErr = fmt.Errorf("close upload reader: %w", closeErr)
	} else {
		closeErr = nil
	}

	return errors.Join(closeErr, <-result)
}
