package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"server/log"
)

// shutdown godoc
// @Summary		Shuts down server
// @Description	Gracefully shuts down server after 1 second.
//
// @Tags			API
//
// @Success		200
// @Router			/shutdown [get].
func shutdown(c *gin.Context) {
	reasonStr := strings.ReplaceAll(c.Param("reason"), "/", "")

	deps, ok := systemDepsFromContext(c)
	if !ok {
		return
	}

	if deps.Settings.ReadOnly() && reasonStr == "" {
		abortAPIError(c, http.StatusForbidden, newForbiddenError("read-only mode requires explicit shutdown reason"))

		return
	}

	if err := authorizeShutdownRequest(c); err != nil {
		if apiErr, ok := err.(APIError); ok && apiErr.Status == http.StatusUnauthorized {
			c.Header("WWW-Authenticate", "Basic realm=Authorization Required")
		}

		abortAPIError(c, http.StatusForbidden, err)

		return
	}

	c.Status(200)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.TLogln("shutdown goroutine panic recovered", "panic", r)
			}
		}()
		time.Sleep(time.Second)

		if RequestShutdown() {
			return
		}

		deps.System.Shutdown()
	}()
}
