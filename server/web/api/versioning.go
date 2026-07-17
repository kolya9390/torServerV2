package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	apiProduct              = "torrserver"
	apiCurrentVersion       = "v1"
	apiManagementCapability = "management-api-v1"
	apiSunsetDateUTC        = "Tue, 30 Jun 2026 23:59:59 GMT"
	apiDeprecationTS        = "@1748736000" // 2025-06-01T00:00:00Z
	apiMigrationDoc         = "/docs/API_VERSIONING.md"
)

type versionDocument struct {
	Product            string   `json:"product"`
	ApplicationVersion string   `json:"application_version"`
	Current            string   `json:"current"`
	Capabilities       []string `json:"capabilities"`
	Deprecated         []string `json:"deprecated,omitempty"`
	Deprecation        string   `json:"deprecation,omitempty"`
	Sunset             string   `json:"sunset,omitempty"`
}

func legacyDeprecationHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Deprecation", apiDeprecationTS)
		c.Header("Sunset", apiSunsetDateUTC)
		c.Header("Link", "<"+apiMigrationDoc+`>; rel="deprecation"`)
		c.Next()
	}
}

func apiVersion(applicationVersion string) gin.HandlerFunc {
	document := versionDocument{
		Product:            apiProduct,
		ApplicationVersion: applicationVersion,
		Current:            apiCurrentVersion,
		Capabilities:       []string{apiManagementCapability},
		Deprecated:         []string{"legacy-root"},
		Deprecation:        time.Unix(1748736000, 0).UTC().Format(time.RFC3339),
		Sunset:             apiSunsetDateUTC,
	}

	return func(c *gin.Context) {
		c.JSON(http.StatusOK, document)
	}
}
