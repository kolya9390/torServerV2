package api

import (
	"net/http"

	"server/internal/app/contracts"
	"server/log"

	"github.com/gin-gonic/gin"
)

/*
file index starts from 1
*/

// viewed godoc
//
//	@Summary		Set / List / Remove viewed torrents
//	@Description	Allow to set, list or remove viewed torrents from server.
//
//	@Tags			API
//
//	@Param			request	body	viewedReqJS	true	"Viewed torrent request. Available params for action: set, rem, list"
//
//	@Accept			json
//	@Produce		json
//	@Success		200 {array} contracts.ViewedItem
//	@Router			/viewed [post]
func viewed(c *gin.Context) {
	req, err := bindViewedRequest(c)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	deps, ok := viewedDepsFromContext(c)
	if !ok {
		return
	}

	switch req.Action {
	case "set":
		setViewed(deps, req, c)
	case "rem":
		remViewed(deps, req, c)
	case "list":
		listViewed(deps, req, c)
	default:
		abortAPIError(c, http.StatusBadRequest, newValidationError("action", "must be one of: set, rem, list"))
	}
}

func setViewed(deps viewedHandlerDeps, req viewedReqJS, c *gin.Context) {
	if deps.Viewed == nil {
		abortAPIError(c, http.StatusInternalServerError, newInternalError("viewed service is unavailable", nil))

		return
	}

	if req.ViewedItem == nil {
		abortAPIError(c, http.StatusBadRequest, newValidationError("viewed", "is required for action=set"))

		return
	}

	deps.Viewed.SetViewed(req.ViewedItem)
	c.Status(200)
}

func remViewed(deps viewedHandlerDeps, req viewedReqJS, c *gin.Context) {
	if deps.Viewed == nil {
		abortAPIError(c, http.StatusInternalServerError, newInternalError("viewed service is unavailable", nil))

		return
	}

	if req.ViewedItem == nil {
		abortAPIError(c, http.StatusBadRequest, newValidationError("viewed", "is required for action=rem"))

		return
	}

	deps.Viewed.RemoveViewed(req.ViewedItem)
	c.Status(200)
}

func listViewed(deps viewedHandlerDeps, req viewedReqJS, c *gin.Context) {
	log.Debug("listViewed: start", "viewed_service_nil", deps.Viewed == nil)

	if deps.Viewed == nil {
		abortAPIError(c, http.StatusInternalServerError, newInternalError("viewed service is unavailable", nil))

		return
	}

	list := deps.Viewed.ListViewed(req.Hash)
	if list == nil {
		list = []*contracts.ViewedItem{}
	}

	log.Debug("listViewed: got list", "items", len(list))
	writeViewedListResponse(c, list)
}
