package api

import (
	"net/http"

	"server/internal/app/contracts"

	"github.com/gin-gonic/gin"
)

func writeMediaProbeResponse(c *gin.Context, data contracts.MediaProbe) {
	c.JSON(http.StatusOK, data)
}
