package apiclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode"
)

const (
	maxErrorRunes = 1024
	maxFieldRunes = 128
	maxCodeRunes  = 64
)

var (
	credentialURLPattern = regexp.MustCompile(`(?i)(https?://)[^/@\s]+@`)
	bearerTokenPattern   = regexp.MustCompile(`(?i)bearer\s+[^\s]+`)
	secretValuePattern   = regexp.MustCompile(`(?i)(--(?:pass|password|token)(?:=|\s+)|(?:pass|password|token)=)[^\s&]+`)
)

// ResponseError is a structured error returned by the TorrServer HTTP API.
type ResponseError struct {
	StatusCode int
	Type       string
	Message    string
	Field      string
	RequestID  string
}

// ResponseLimitError reports a response body that exceeded the client bound.
type ResponseLimitError struct {
	Limit int64
}

// ResponseDecodeError reports a successful HTTP response with an invalid JSON document.
type ResponseDecodeError struct {
	Err error
}

func (err *ResponseLimitError) Error() string {
	return fmt.Sprintf("response body exceeds the %d-byte CLI limit", err.Limit)
}

func (err *ResponseDecodeError) Error() string {
	return fmt.Sprintf("decode response: %v", err.Err)
}

func (err *ResponseDecodeError) Unwrap() error {
	return err.Err
}

func (err *ResponseError) Error() string {
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

type apiErrorEnvelope struct {
	Error     json.RawMessage `json:"error"`
	RequestID string          `json:"request_id,omitempty"`
}

type apiErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func parseResponseError(statusCode int, data []byte) error {
	if len(data) == 0 {
		return newResponseError(statusCode, "api_error", http.StatusText(statusCode), "", "")
	}

	var envelope apiErrorEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return newResponseError(statusCode, "api_error", "server returned a non-JSON error response", "", "")
	}

	if len(envelope.Error) == 0 || string(envelope.Error) == "null" {
		return newResponseError(
			statusCode,
			"api_error",
			"server returned an error without a message",
			"",
			envelope.RequestID,
		)
	}

	var detail apiErrorDetail
	if err := json.Unmarshal(envelope.Error, &detail); err == nil {
		return responseErrorFromDetail(statusCode, detail, envelope.RequestID)
	}

	var message string
	if err := json.Unmarshal(envelope.Error, &message); err == nil && strings.TrimSpace(message) != "" {
		return newResponseError(statusCode, "api_error", message, "", envelope.RequestID)
	}

	return newResponseError(statusCode, "api_error", "server returned an invalid error response", "", envelope.RequestID)
}

func responseErrorFromDetail(statusCode int, detail apiErrorDetail, requestID string) error {
	message := detail.Message
	if strings.TrimSpace(message) == "" {
		message = "server returned an error without a message"
	}

	errType := detail.Type
	if strings.TrimSpace(errType) == "" {
		errType = "api_error"
	}

	return newResponseError(statusCode, errType, message, detail.Field, requestID)
}

func newResponseError(statusCode int, errType, message, field, requestID string) *ResponseError {
	return &ResponseError{
		StatusCode: statusCode,
		Type:       sanitizeErrorCode(errType),
		Message:    sanitizeErrorText(message),
		Field:      sanitizeErrorField(field),
		RequestID:  sanitizeErrorField(requestID),
	}
}

func sanitizeErrorText(value string) string {
	value = credentialURLPattern.ReplaceAllString(value, `${1}[redacted]@`)
	value = bearerTokenPattern.ReplaceAllString(value, "Bearer [redacted]")
	value = secretValuePattern.ReplaceAllStringFunc(value, redactSecretValue)
	value = strings.Map(func(char rune) rune {
		if unicode.IsControl(char) {
			return ' '
		}

		return char
	}, value)
	value = strings.Join(strings.Fields(value), " ")

	runes := []rune(value)
	if len(runes) > maxErrorRunes {
		value = string(runes[:maxErrorRunes]) + "..."
	}

	if value == "" {
		return "unknown error"
	}

	return value
}

func sanitizeErrorField(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}

	runes := []rune(sanitizeErrorText(value))
	if len(runes) > maxFieldRunes {
		return string(runes[:maxFieldRunes])
	}

	return string(runes)
}

func sanitizeErrorCode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Map(func(char rune) rune {
		switch {
		case char >= 'a' && char <= 'z':
			return char
		case char >= '0' && char <= '9':
			return char
		case char == '_', char == '-':
			return char
		default:
			return -1
		}
	}, value)

	runes := []rune(value)
	if len(runes) > maxCodeRunes {
		value = string(runes[:maxCodeRunes])
	}

	if value == "" {
		return "api_error"
	}

	return value
}

func redactSecretValue(value string) string {
	separator := strings.IndexAny(value, "= \t")
	if separator < 0 {
		return "[redacted]"
	}

	return value[:separator+1] + "[redacted]"
}
