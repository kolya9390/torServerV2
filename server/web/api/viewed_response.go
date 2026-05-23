package api

import (
	"net/http"

	"server/internal/app/contracts"

	"github.com/gin-gonic/gin"
)

func writeViewedListResponse(c *gin.Context, list []*contracts.ViewedItem) {
	c.JSON(http.StatusOK, list)
}
