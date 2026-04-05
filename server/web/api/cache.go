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
//	@Success		200	{object} state.CacheState	"Cache stats"
//	@Router			/cache [post]
func cache(c *gin.Context) {
	svc := getServices()

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
		getCache(svc, req, c)
	default:
		abortAPIError(c, http.StatusBadRequest, newValidationError("action", "must be one of: get"))
	}
}

func getCache(svc *APIServices, req cacheReqJS, c *gin.Context) {
	if req.Hash == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("hash", "is required for action=get"))

		return
	}

	tor := svc.Torrents.Get(req.Hash)

	if tor != nil {
		st := tor.CacheState()
		if st == nil {
			c.JSON(200, struct{}{})
		} else {
			c.JSON(200, st)
		}
	} else {
		abortAPIError(c, http.StatusNotFound, newNotFoundError("torrent not found"))
	}
}
