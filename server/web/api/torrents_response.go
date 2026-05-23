package api

import (
	"net/http"

	"server/internal/app/contracts"

	"github.com/gin-gonic/gin"
)

func writeTorrentStatusResponse(c *gin.Context, status *contracts.TorrentStatus) {
	c.JSON(http.StatusOK, status)
}

func writeTorrentStatusListResponse(c *gin.Context, statuses []*contracts.TorrentStatus) {
	c.JSON(http.StatusOK, statuses)
}
