package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
//	@Success		200	{array}	contracts.SearchResult	"Torznab torrent search result(s)"
//	@Router			/torznab/search [get]
func torznabSearch(c *gin.Context) {
	deps, ok := searchDepsFromContext(c)
	if !ok {
		return
	}

	if !deps.Search.EnableTorznabSearch() {
		writeTorznabDisabledResponse(c)

		return
	}

	searchReq, err := bindTorznabSearchRequest(c)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	writeTorznabSearchResponse(c, deps.Search.TorznabSearch(searchReq.Query, searchReq.Index))
}

func torznabTest(c *gin.Context) {
	deps, ok := searchDepsFromContext(c)
	if !ok {
		return
	}

	req, err := bindTorznabTestRequest(c)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	if err := deps.Search.TorznabTest(req.Host, req.Key); err != nil {
		writeTorznabTestFailure(c, err)

		return
	}

	writeTorznabTestSuccess(c)
}
