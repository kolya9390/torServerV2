package cli

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
	"strings"
	"time"
)

const (
	defaultMaxCLIResponseBytes      int64 = 16 << 20
	defaultMaxCLIErrorResponseBytes int64 = 64 << 10
)

type apiClient struct {
	baseURL          *url.URL
	http             *http.Client
	user             string
	pass             string
	maxResponse      int64
	maxErrorResponse int64
}

type apiErrorBody struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Field   string `json:"field,omitempty"`
		Cause   string `json:"cause,omitempty"`
	} `json:"error"`
	RequestID string `json:"request_id,omitempty"`
}

type apiResponseError struct {
	StatusCode int
	Type       string
	Message    string
	Field      string
	RequestID  string
}

type responseLimitError struct {
	Limit int64
}

func (err *responseLimitError) Error() string {
	return fmt.Sprintf("response body exceeds the %d-byte CLI limit", err.Limit)
}

func (err *apiResponseError) Error() string {
	message := sanitizeErrorText(err.Message)
	if field := sanitizeErrorField(err.Field); field != "" {
		message = field + ": " + message
	}

	if requestID := sanitizeErrorField(err.RequestID); requestID != "" {
		message += " (request_id=" + requestID + ")"
	}

	return fmt.Sprintf(
		"api error: status=%d type=%s message=%s",
		err.StatusCode,
		sanitizeErrorCode(err.Type),
		message,
	)
}

func newAPIClient(opts globalOptions) (*apiClient, error) {
	parsed, err := normalizeServerURL(opts.Server)
	if err != nil {
		return nil, err
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("default HTTP transport has an unsupported implementation")
	}

	transport := defaultTransport.Clone()
	transport.TLSClientConfig = &tls.Config{ //nolint:gosec // InsecureSkipVerify is an explicit CLI opt-in.
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: opts.Insecure,
	}

	return &apiClient{
		baseURL: parsed,
		http: &http.Client{
			Timeout:       timeout,
			Transport:     transport,
			CheckRedirect: rejectRedirect,
		},
		user:             opts.User,
		pass:             opts.Pass,
		maxResponse:      defaultMaxCLIResponseBytes,
		maxErrorResponse: defaultMaxCLIErrorResponseBytes,
	}, nil
}

func normalizeServerURL(value string) (*url.URL, error) {
	rawBase := strings.TrimSpace(value)
	if rawBase == "" {
		rawBase = defaultServerURL
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

func (c *apiClient) doJSON(ctx context.Context, method, path string, requestBody any, responseBody any, headers map[string]string) error {
	var bodyReader io.Reader

	if requestBody != nil {
		payload, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}

		bodyReader = bytes.NewReader(payload)
	}

	req, err := c.newRequest(ctx, method, path, bodyReader)
	if err != nil {
		return err
	}

	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	req.Header.Set("Accept", "application/json")

	for key, val := range headers {
		if strings.TrimSpace(val) != "" {
			req.Header.Set(key, val)
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	data, err := c.readAndCloseResponse(resp)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return parseAPIError(resp.StatusCode, data)
	}

	if responseBody == nil || len(data) == 0 {
		return nil
	}

	if err := json.Unmarshal(data, responseBody); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

func (c *apiClient) doMultipartFile(
	ctx context.Context,
	path string,
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

	req, err := c.newRequest(ctx, http.MethodPost, path, reader)
	if err != nil {
		return errors.Join(err, finishMultipartUpload(reader, err, writeErr))
	}

	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		requestErr := fmt.Errorf("request failed: %w", err)

		return errors.Join(requestErr, finishMultipartUpload(reader, requestErr, writeErr))
	}

	data, readErr := c.readAndCloseResponse(resp)
	uploadErr := finishMultipartUpload(reader, nil, writeErr)

	if readErr != nil {
		return errors.Join(fmt.Errorf("read response: %w", readErr), uploadErr)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return errors.Join(parseAPIError(resp.StatusCode, data), uploadErr)
	}

	if uploadErr != nil {
		return fmt.Errorf("upload torrent file: %w", uploadErr)
	}

	if responseBody == nil || len(data) == 0 {
		return nil
	}

	if err := json.Unmarshal(data, responseBody); err != nil {
		return fmt.Errorf("decode response: %w", err)
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

	for key, val := range fields {
		if val == "" {
			continue
		}

		if err := writer.WriteField(key, val); err != nil {
			return fmt.Errorf("write multipart field %s: %w", key, err)
		}
	}

	return nil
}

func (c *apiClient) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	fullURL, err := c.resolveEndpoint(path)
	if err != nil {
		return nil, fmt.Errorf("build request url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if c.user != "" || c.pass != "" {
		req.SetBasicAuth(c.user, c.pass)
	}

	return req, nil
}

func (c *apiClient) resolveEndpoint(endpoint string) (*url.URL, error) {
	if c == nil || c.baseURL == nil {
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

	return c.baseURL.ResolveReference(relative), nil
}

func (c *apiClient) readAndCloseResponse(response *http.Response) ([]byte, error) {
	if response == nil || response.Body == nil {
		return nil, errors.New("HTTP response body is missing")
	}

	limit := c.responseLimit(response.StatusCode)
	data, readErr := readBounded(response.Body, limit)

	if closeErr := response.Body.Close(); closeErr != nil {
		readErr = errors.Join(readErr, fmt.Errorf("close response: %w", closeErr))
	}

	return data, readErr
}

func (c *apiClient) responseLimit(statusCode int) int64 {
	if statusCode >= http.StatusBadRequest {
		if c.maxErrorResponse > 0 {
			return c.maxErrorResponse
		}

		return defaultMaxCLIErrorResponseBytes
	}

	if c.maxResponse > 0 {
		return c.maxResponse
	}

	return defaultMaxCLIResponseBytes
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}

	if int64(len(data)) > limit {
		return nil, &responseLimitError{Limit: limit}
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

func parseAPIError(statusCode int, data []byte) error {
	if len(data) == 0 {
		return &apiResponseError{
			StatusCode: statusCode,
			Type:       "api_error",
			Message:    http.StatusText(statusCode),
		}
	}

	var apiErr apiErrorBody
	if err := json.Unmarshal(data, &apiErr); err != nil {
		return &apiResponseError{
			StatusCode: statusCode,
			Type:       "api_error",
			Message:    "server returned a non-JSON error response",
		}
	}

	if apiErr.Error.Message == "" {
		return &apiResponseError{
			StatusCode: statusCode,
			Type:       "api_error",
			Message:    "server returned an error without a message",
			RequestID:  apiErr.RequestID,
		}
	}

	return &apiResponseError{
		StatusCode: statusCode,
		Type:       firstNonEmpty(apiErr.Error.Type, "api_error"),
		Message:    apiErr.Error.Message,
		Field:      apiErr.Error.Field,
		RequestID:  apiErr.RequestID,
	}
}
