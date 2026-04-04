package api

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"

	"server/torznab"
)

// torznabSearch godoc
//
//	@Summary		Makes a torznab search
//	@Description	Makes a torznab search.
//
//	@Tags			API
//
//	@Param			query	query	string	true	"Torznab query"
//
//	@Produce		json
//	@Success		200	{array}	torznab.TorrentDetails	"Torznab torrent search result(s)"
//	@Router			/torznab/search [get]
func torznabSearch(c *gin.Context) {
	svc := getServices()
	if !svc.Search.EnableTorznabSearch() {
		c.JSON(http.StatusBadRequest, []string{})
		return
	}
	query := c.Query("query")
	indexStr := c.DefaultQuery("index", "-1")
	index := -1
	if i, err := strconv.Atoi(indexStr); err == nil {
		index = i
	}

	decodedQuery, err := url.QueryUnescape(query)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, newValidationError("query", "invalid query encoding"))
		return
	}
	query = decodedQuery
	list := svc.Search.TorznabSearch(query, index)
	if list == nil {
		list = []*torznab.TorrentDetails{}
	}
	c.JSON(200, list)
}

type torznabTestReq struct {
	Host string `json:"host"`
	Key  string `json:"key"`
}

func torznabTest(c *gin.Context) {
	var req torznabTestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abortAPIError(c, http.StatusBadRequest, newValidationError("request", "invalid json body"))
		return
	}
	if req.Host == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("host", "is required"))
		return
	}

	if err := getServices().Search.TorznabTest(req.Host, req.Key); err != nil {
		c.JSON(200, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true})
}
