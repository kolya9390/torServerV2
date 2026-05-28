package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

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
	req, err := bindCacheRequest(c)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	deps, ok := cacheDepsFromContext(c)
	if !ok {
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
		writeCacheStateResponse(c, st)
	} else {
		abortAPIError(c, http.StatusNotFound, newNotFoundError("torrent not found"))
	}
}
