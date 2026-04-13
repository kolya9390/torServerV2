package cli

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

	defer func() {
		_ = resp.Body.Close()
	}()

	data, err := io.ReadAll(resp.Body)
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
