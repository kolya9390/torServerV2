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
	"path/filepath"
	"strings"
	"time"
)

type apiClient struct {
	baseURL *url.URL
	http    *http.Client
	user    string
	pass    string
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

func newAPIClient(opts globalOptions) (*apiClient, error) {
	rawBase := strings.TrimSpace(opts.Server)
	if rawBase == "" {
		rawBase = "http://127.0.0.1:8090"
	}

	if !strings.Contains(rawBase, "://") {
		rawBase = "http://" + rawBase
	}

	parsed, err := url.Parse(strings.TrimRight(rawBase, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid --server value: %w", err)
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	return &apiClient{
		baseURL: parsed,
		http: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: opts.Insecure}, //nolint:gosec // explicit CLI option
			},
		},
		user: opts.User,
		pass: opts.Pass,
	}, nil
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

	data, err := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); closeErr != nil {
		err = errors.Join(err, fmt.Errorf("close response: %w", closeErr))
	}

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
		_ = reader.CloseWithError(err)

		<-writeErr

		return err
	}

	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		_ = reader.CloseWithError(err)

		<-writeErr

		return fmt.Errorf("request failed: %w", err)
	}

	data, readErr := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); closeErr != nil {
		readErr = errors.Join(readErr, fmt.Errorf("close response: %w", closeErr))
	}

	_ = reader.Close()
	uploadErr := <-writeErr

	if readErr != nil {
		return fmt.Errorf("read response: %w", readErr)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return parseAPIError(resp.StatusCode, data)
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
	fullURL, err := c.baseURL.Parse(path)
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

func parseAPIError(statusCode int, data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("api error: status=%d", statusCode)
	}

	var apiErr apiErrorBody
	if err := json.Unmarshal(data, &apiErr); err != nil {
		return fmt.Errorf("api error: status=%d body=%s", statusCode, strings.TrimSpace(string(data)))
	}

	if apiErr.Error.Message == "" {
		return fmt.Errorf("api error: status=%d body=%s", statusCode, strings.TrimSpace(string(data)))
	}

	msg := apiErr.Error.Message
	if apiErr.Error.Field != "" {
		msg = fmt.Sprintf("%s: %s", apiErr.Error.Field, apiErr.Error.Message)
	}

	if apiErr.RequestID != "" {
		msg = fmt.Sprintf("%s (request_id=%s)", msg, apiErr.RequestID)
	}

	return fmt.Errorf("api error: status=%d type=%s message=%s", statusCode, apiErr.Error.Type, msg)
}
