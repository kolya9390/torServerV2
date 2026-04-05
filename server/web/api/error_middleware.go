package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"server/log"
)

func ErrorResponder() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic in handler", "request_id", log.GetRequestID(c), "panic", r, "path", c.FullPath())

				body := gin.H{
					"error": gin.H{
						"type":    "internal_error",
						"message": "internal server error",
					},
				}
				if rid := log.GetRequestID(c); rid != "" {
					body["request_id"] = rid
				}

				c.AbortWithStatusJSON(http.StatusInternalServerError, body)
			}
		}()

		c.Next()

		if len(c.Errors) == 0 {
			return
		}

		if c.Writer.Size() > 0 {
			return
		}

		lastErr := c.Errors.Last().Err

		status := c.Writer.Status()
		if status < http.StatusBadRequest {
			status = http.StatusInternalServerError
		}

		body := buildErrorBody(c, lastErr, status)
		logError(c, status, lastErr)

		c.AbortWithStatusJSON(status, body)
	}
}

func buildErrorBody(c *gin.Context, err error, status int) gin.H {
	body := gin.H{
		"error": gin.H{
			"type":    "error",
			"message": err.Error(),
		},
	}

	if apiErr, ok := err.(APIError); ok {
		if apiErr.Status > 0 {
			body["error"] = gin.H{
				"type":    apiErr.Type,
				"message": apiErr.Message,
			}
		}

		errBody := gin.H{
			"type":    apiErr.Type,
			"message": apiErr.Message,
		}
		if apiErr.Field != "" {
			errBody["field"] = apiErr.Field
		}

		if apiErr.Cause != nil {
			errBody["cause"] = apiErr.Cause.Error()
		}

		body["error"] = errBody
	}

	if rid := log.GetRequestID(c); rid != "" {
		body["request_id"] = rid
	}

	return body
}

func logError(c *gin.Context, status int, err error) {
	requestID := log.GetRequestID(c)
	path := c.FullPath()

	switch {
	case status >= http.StatusInternalServerError:
		log.Error("api_error",
			"request_id", requestID,
			"status", status,
			"path", path,
			"err", err,
		)
	case status >= http.StatusBadRequest:
		log.Warn("api_error",
			"request_id", requestID,
			"status", status,
			"path", path,
			"err", err,
		)
	default:
		log.Info("api_error",
			"request_id", requestID,
			"status", status,
			"path", path,
			"err", err,
		)
	}
}
