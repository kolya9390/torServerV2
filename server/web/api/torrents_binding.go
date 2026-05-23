package api

import "github.com/gin-gonic/gin"

// Action: add, get, set, rem, list, drop.
type torrReqJS struct {
	requestI
	Link     string `json:"link,omitempty"`
	Hash     string `json:"hash,omitempty"`
	Title    string `json:"title,omitempty"`
	Category string `json:"category,omitempty"`
	Poster   string `json:"poster,omitempty"`
	Data     string `json:"data,omitempty"`
	SaveToDB bool   `json:"save_to_db,omitempty"`
}

func bindTorrentRequest(c *gin.Context) (torrReqJS, error) {
	var req torrReqJS
	if err := c.ShouldBindJSON(&req); err != nil {
		return torrReqJS{}, newValidationError("request", "invalid json body")
	}

	if req.Action == "" {
		return torrReqJS{}, newValidationError("action", "is required")
	}

	return req, nil
}
