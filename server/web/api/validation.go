package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"

	"server/log"
	sets "server/settings"
)

type APIError struct {
	Type    string
	Message string
	Status  int
	Field   string
	Cause   error
}

func (e APIError) Error() string {
	if e.Field != "" {
		return e.Field + ": " + e.Message
	}
	return e.Message
}

func newValidationError(field, message string) error {
	return APIError{
		Type:    "validation_error",
		Message: message,
		Status:  http.StatusBadRequest,
		Field:   field,
	}
}

func newUnauthorizedError(message string) error {
	return APIError{
		Type:    "unauthorized",
		Message: message,
		Status:  http.StatusUnauthorized,
	}
}

func newNotFoundError(message string) error {
	return APIError{
		Type:    "not_found",
		Message: message,
		Status:  http.StatusNotFound,
	}
}

func newConflictError(message string) error {
	return APIError{
		Type:    "conflict",
		Message: message,
		Status:  http.StatusConflict,
	}
}

func newInternalError(message string, cause error) error {
	return APIError{
		Type:    "internal_error",
		Message: message,
		Status:  http.StatusInternalServerError,
		Cause:   cause,
	}
}

func abortAPIError(c *gin.Context, fallbackStatus int, err error) {
	status := fallbackStatus
	body := gin.H{
		"error": gin.H{
			"type":    "error",
			"message": err.Error(),
		},
	}

	if apiErr, ok := err.(APIError); ok {
		if apiErr.Status > 0 {
			status = apiErr.Status
		}
		e := gin.H{
			"type":    apiErr.Type,
			"message": apiErr.Message,
		}
		if apiErr.Field != "" {
			e["field"] = apiErr.Field
		}
		if apiErr.Cause != nil {
			e["cause"] = apiErr.Cause.Error()
		}
		body["error"] = e
	}

	if rid := log.GetRequestID(c); rid != "" {
		body["request_id"] = rid
	}
	c.AbortWithStatusJSON(status, body)
}

func generateSettingsETag(s *sets.BTSets) string {
	data := []byte{
		byte(s.CacheSize >> 24), byte(s.CacheSize >> 16), byte(s.CacheSize >> 8), byte(s.CacheSize),
		byte(s.TorrentDisconnectTimeout >> 8), byte(s.TorrentDisconnectTimeout),
		byte(s.PreloadCache),
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:8])
}
