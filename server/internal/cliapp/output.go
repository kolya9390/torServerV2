package cliapp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"
	"unicode"

	"server/internal/apiclient"
)

const maxCLIErrorRunes = 1024

var (
	credentialURLPattern = regexp.MustCompile(`(?i)(https?://)[^/@\s]+@`)
	bearerTokenPattern   = regexp.MustCompile(`(?i)bearer\s+[^\s]+`)
	secretValuePattern   = regexp.MustCompile(`(?i)(--(?:pass|password|token)(?:=|\s+)|(?:pass|password|token)=)[^\s&]+`)
)

type successEnvelope struct {
	OK   bool `json:"ok"`
	Data any  `json:"data"`
}

type errorEnvelope struct {
	OK    bool            `json:"ok"`
	Error cliErrorPayload `json:"error"`
}

type cliErrorPayload struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Status    int    `json:"status,omitempty"`
	Field     string `json:"field,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

func writeJSONSuccess(writer io.Writer, data any) error {
	return writeJSON(writer, successEnvelope{OK: true, Data: data})
}

func writeCommandResult(opts globalOptions, data any, humanMessage string) error {
	if opts.Output == outputJSON {
		return writeJSONSuccess(opts.stdoutWriter(), data)
	}

	return writeTextLine(opts.stdoutWriter(), humanMessage)
}

func writeTextLine(writer io.Writer, value string) error {
	//nolint:gosec // CLI terminal/file writer is not an HTML sink; control characters are sanitized.
	_, err := io.WriteString(writer, sanitizeTerminalText(value)+"\n")

	return err
}

func writeTextLines(writer io.Writer, values ...string) error {
	for _, value := range values {
		if err := writeTextLine(writer, value); err != nil {
			return err
		}
	}

	return nil
}

func writeJSONError(writer io.Writer, err error) error {
	return writeJSON(writer, errorEnvelope{OK: false, Error: errorPayload(err)})
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	return encoder.Encode(value)
}

func errorPayload(err error) cliErrorPayload {
	payload := cliErrorPayload{
		Code:    "command_error",
		Message: sanitizeErrorMessage(err),
	}

	var compatibilityErr *apiclient.CompatibilityError
	if errors.As(err, &compatibilityErr) {
		payload.Code = string(compatibilityErr.Kind)

		var apiErr *apiclient.ResponseError
		if errors.As(err, &apiErr) {
			payload.Status = apiErr.StatusCode
			payload.Field = sanitizeErrorField(apiErr.Field)
			payload.RequestID = sanitizeErrorField(apiErr.RequestID)
		}

		return payload
	}

	var apiErr *apiclient.ResponseError
	if errors.As(err, &apiErr) {
		payload.Code = sanitizeErrorCode(apiErr.Type)
		payload.Message = sanitizeErrorText(apiErr.Message)
		payload.Status = apiErr.StatusCode
		payload.Field = sanitizeErrorField(apiErr.Field)
		payload.RequestID = sanitizeErrorField(apiErr.RequestID)

		return payload
	}

	var urlErr *url.Error

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		payload.Code = "timeout"
	case errors.Is(err, context.Canceled):
		payload.Code = "canceled"
	case errors.As(err, &urlErr):
		payload.Code = "network_error"
	case errors.Is(err, os.ErrNotExist), errors.Is(err, os.ErrPermission):
		payload.Code = "io_error"
	}

	return payload
}

func sanitizeErrorMessage(err error) string {
	if err == nil {
		return "unknown error"
	}

	return sanitizeErrorText(err.Error())
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
	if len(runes) > maxCLIErrorRunes {
		value = string(runes[:maxCLIErrorRunes]) + "..."
	}

	if value == "" {
		return "unknown error"
	}

	return value
}

func sanitizeErrorField(value string) string {
	const maxFieldRunes = 128

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
	const maxCodeRunes = 64

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

func redactURLCredentials(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return sanitizeErrorText(value)
	}

	parsed.User = nil

	query := parsed.Query()
	for key := range query {
		switch strings.ToLower(key) {
		case "pass", "password", "token", "access_token":
			query.Set(key, "[redacted]")
		}
	}

	parsed.RawQuery = query.Encode()

	return parsed.String()
}

func sanitizeTerminalText(value string) string {
	return strings.Map(func(char rune) rune {
		if unicode.IsControl(char) {
			return '?'
		}

		return char
	}, value)
}

func requestedJSONOutput(args []string) bool {
	for index, arg := range args {
		if value, found := strings.CutPrefix(arg, "--output="); found {
			if strings.EqualFold(strings.TrimSpace(value), outputJSON) {
				return true
			}

			continue
		}

		if arg == "--output" && index+1 < len(args) {
			return strings.EqualFold(strings.TrimSpace(args[index+1]), outputJSON)
		}
	}

	return false
}
