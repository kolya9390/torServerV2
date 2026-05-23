package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func writeCacheStateResponse(c *gin.Context, state any) {
	if state == nil {
		c.JSON(http.StatusOK, struct{}{})

		return
	}

	c.JSON(http.StatusOK, state)
}
