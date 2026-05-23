package api

import (
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"
)

type torznabSearchReq struct {
	Query string
	Index int
}

type torznabTestReq struct {
	Host string `json:"host"`
	Key  string `json:"key"`
}

func bindTorznabSearchRequest(c *gin.Context) (torznabSearchReq, error) {
	index := -1
	if i, err := strconv.Atoi(c.DefaultQuery("index", "-1")); err == nil {
		index = i
	}

	query, err := url.QueryUnescape(c.Query("query"))
	if err != nil {
		return torznabSearchReq{}, newValidationError("query", "invalid query encoding")
	}

	return torznabSearchReq{
		Query: query,
		Index: index,
	}, nil
}

func bindTorznabTestRequest(c *gin.Context) (torznabTestReq, error) {
	var req torznabTestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		return torznabTestReq{}, newValidationError("request", "invalid json body")
	}

	if req.Host == "" {
		return torznabTestReq{}, newValidationError("host", "is required")
	}

	return req, nil
}
