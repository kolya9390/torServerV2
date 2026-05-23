package api

import (
	"encoding/json"
	"net/http"

	"server/internal/app/contracts"

	"github.com/gin-gonic/gin"
)

func writeSettingsResponse(c *gin.Context, current *contracts.Settings) {
	etag := generateSettingsETag(current)
	if match := c.GetHeader("If-None-Match"); match == etag {
		c.Status(http.StatusNotModified)

		return
	}

	c.Header("ETag", etag)
	c.Header("Cache-Control", "private, max-age=5")
	c.Header("Content-Type", "application/json")

	data, err := json.Marshal(current)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal settings"})

		return
	}

	c.Data(http.StatusOK, "application/json", data)
}
