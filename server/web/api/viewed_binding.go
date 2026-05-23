package api

import (
	"server/internal/app/contracts"

	"github.com/gin-gonic/gin"
)

// Action: set, rem, list.
type viewedReqJS struct {
	requestI
	*contracts.ViewedItem
}

func bindViewedRequest(c *gin.Context) (viewedReqJS, error) {
	var req viewedReqJS
	if err := c.ShouldBindJSON(&req); err != nil {
		return viewedReqJS{}, newValidationError("request", "invalid json body")
	}

	if req.Action == "" {
		return viewedReqJS{}, newValidationError("action", "is required")
	}

	return req, nil
}
