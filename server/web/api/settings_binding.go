package api

import (
	"bytes"
	"encoding/json"

	"server/internal/app/contracts"

	"github.com/gin-gonic/gin"
)

// Action: get, set, def.
type setsReqJS struct {
	requestI
	Sets    *contracts.Settings `json:"sets,omitempty"`
	SetsRaw json.RawMessage     `json:"-"`
}

func (r *setsReqJS) UnmarshalJSON(data []byte) error {
	var req struct {
		Action string          `json:"action,omitempty"`
		Sets   json.RawMessage `json:"sets,omitempty"`
	}

	if err := json.Unmarshal(data, &req); err != nil {
		return err
	}

	r.Action = req.Action
	r.SetsRaw = nil
	r.Sets = nil

	if !hasRawJSONValue(req.Sets) {
		return nil
	}

	var settings contracts.Settings
	if err := json.Unmarshal(req.Sets, &settings); err != nil {
		return err
	}

	r.Sets = &settings
	r.SetsRaw = append(r.SetsRaw[:0], req.Sets...)

	return nil
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

func hasRawJSONValue(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)

	return len(raw) > 0 && !bytes.Equal(raw, []byte("null"))
}
