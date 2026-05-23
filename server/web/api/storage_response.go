package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func writeStoragePreferencesResponse(c *gin.Context, prefs map[string]any) {
	c.JSON(http.StatusOK, prefs)
}

func writeStorageUpdateResponse(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
