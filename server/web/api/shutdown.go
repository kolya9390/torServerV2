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
// @Router			/shutdown [get]
func shutdown(c *gin.Context) {
	svc := getServices()
	reasonStr := strings.ReplaceAll(c.Param("reason"), "/", "")
	if svc.Settings.ReadOnly() && reasonStr == "" {
		c.Status(http.StatusForbidden)
		return
	}
	c.Status(200)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.TLogln("shutdown goroutine panic recovered", "panic", r)
			}
		}()
		time.Sleep(1000)
		getServices().System.Shutdown()
	}()
}
