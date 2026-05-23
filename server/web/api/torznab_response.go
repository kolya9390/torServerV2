package api

import (
	"net/http"

	"server/internal/app/contracts"

	"github.com/gin-gonic/gin"
)

func writeTorznabDisabledResponse(c *gin.Context) {
	c.JSON(http.StatusBadRequest, []string{})
}

func writeTorznabSearchResponse(c *gin.Context, list []*contracts.SearchResult) {
	if list == nil {
		list = []*contracts.SearchResult{}
	}

	c.JSON(http.StatusOK, list)
}

func writeTorznabTestFailure(c *gin.Context, err error) {
	c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
}

func writeTorznabTestSuccess(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true})
}
