package api

import (
	"net/http"

	"server/internal/app/contracts"

	"github.com/gin-gonic/gin"
)

func writeStreamStatusResponse(c *gin.Context, status *contracts.TorrentStatus) {
	c.JSON(http.StatusOK, status)
}

func writeStreamSaveResponse(c *gin.Context, hash string) {
	c.JSON(http.StatusOK, gin.H{"status": "saved", "hash": hash})
}
