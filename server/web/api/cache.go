package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Action: get.
type cacheReqJS struct {
	requestI
	Hash string `json:"hash,omitempty"`
}

// cache godoc
//
//	@Summary		Return cache stats
//	@Description	Return cache stats.
//
//	@Tags			API
//
//	@Param			request	body	cacheReqJS	true	"Cache stats request"
//
//	@Produce		json
//	@Success		200	{object} object	"Cache stats"
//	@Router			/cache [post]
func cache(c *gin.Context) {
	deps := cacheDepsFromContext(c)

	var req cacheReqJS

	err := c.ShouldBindJSON(&req)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, newValidationError("request", "invalid json body"))

		return
	}

	if req.Action == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("action", "is required"))

		return
	}

	switch req.Action {
	case "get":
		getCache(deps, req, c)
	default:
		abortAPIError(c, http.StatusBadRequest, newValidationError("action", "must be one of: get"))
	}
}

func getCache(deps cacheHandlerDeps, req cacheReqJS, c *gin.Context) {
	if req.Hash == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("hash", "is required for action=get"))

		return
	}

	if st, found := deps.Torrents.CacheStateByHash(req.Hash); found {
		if st == nil {
			c.JSON(200, struct{}{})
		} else {
			c.JSON(200, st)
		}
	} else {
		abortAPIError(c, http.StatusNotFound, newNotFoundError("torrent not found"))
	}
}
