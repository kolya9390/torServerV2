package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	apiCurrentVersion = "v1"
	apiSunsetDateUTC  = "Tue, 30 Jun 2026 23:59:59 GMT"
	apiDeprecationTS  = "@1748736000" // 2025-06-01T00:00:00Z
	apiMigrationDoc   = "/docs/API_VERSIONING.md"
)

func legacyDeprecationHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Deprecation", apiDeprecationTS)
		c.Header("Sunset", apiSunsetDateUTC)
		c.Header("Link", "<"+apiMigrationDoc+`>; rel="deprecation"`)
		c.Next()
	}
}

func apiVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"current":     apiCurrentVersion,
		"deprecated":  []string{"legacy-root"},
		"deprecation": time.Unix(1748736000, 0).UTC().Format(time.RFC3339),
		"sunset":      apiSunsetDateUTC,
	})
}
