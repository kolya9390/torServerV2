package api

import (
	"server/internal/app/contracts"

	"github.com/gin-gonic/gin"
)

// Action: get, set, def.
type setsReqJS struct {
	requestI
	Sets *contracts.Settings `json:"sets,omitempty"`
}

func bindSettingsRequest(c *gin.Context) (setsReqJS, error) {
	var req setsReqJS
	if err := c.ShouldBindJSON(&req); err != nil {
		return setsReqJS{}, newValidationError("request", "invalid json body")
	}

	if req.Action == "" {
		return setsReqJS{}, newValidationError("action", "is required")
	}

	if req.Action == "set" && req.Sets == nil {
		return setsReqJS{}, newValidationError("sets", "is required for action=set")
	}

	return req, nil
}
