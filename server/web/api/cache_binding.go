package api

import "github.com/gin-gonic/gin"

// Action: get.
type cacheReqJS struct {
	requestI
	Hash string `json:"hash,omitempty"`
}

func bindCacheRequest(c *gin.Context) (cacheReqJS, error) {
	var req cacheReqJS
	if err := c.ShouldBindJSON(&req); err != nil {
		return cacheReqJS{}, newValidationError("request", "invalid json body")
	}

	if req.Action == "" {
		return cacheReqJS{}, newValidationError("action", "is required")
	}

	return req, nil
}
