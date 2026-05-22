package api

import (
	"net/http"
	"runtime/debug"

	"server/log"
	sets "server/settings"

	"github.com/gin-gonic/gin"
)

/*
file index starts from 1
*/

// Action: set, rem, list.
type viewedReqJS struct {
	requestI
	*sets.Viewed
}

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
//	@Success		200 {array} sets.Viewed
//	@Router			/viewed [post]
func viewed(c *gin.Context) {
	deps := viewedDepsFromContext(c)

	var req viewedReqJS

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
	if deps.Viewed == nil || req.Viewed == nil {
		abortAPIError(c, http.StatusBadRequest, newValidationError("viewed", "is required for action=set"))

		return
	}

	deps.Viewed.SetViewed(req.Viewed)
	c.Status(200)
}

func remViewed(deps viewedHandlerDeps, req viewedReqJS, c *gin.Context) {
	if deps.Viewed == nil || req.Viewed == nil {
		abortAPIError(c, http.StatusBadRequest, newValidationError("viewed", "is required for action=rem"))

		return
	}

	deps.Viewed.RemoveViewed(req.Viewed)
	c.Status(200)
}

func listViewed(deps viewedHandlerDeps, req viewedReqJS, c *gin.Context) {
	log.TLogln("listViewed: START")
	log.TLogln("listViewed: deps.Viewed is nil?", deps.Viewed == nil)

	defer func() {
		if r := recover(); r != nil {
			log.TLogln("listViewed PANIC RECOVERED:", r)
			log.TLogln("stack:", string(debug.Stack()))
			c.JSON(200, []*sets.Viewed{})

			return
		}
	}()

	log.TLogln("listViewed: calling sets.ListViewed directly")

	list := sets.ListViewed(req.Hash)
	log.TLogln("listViewed: got list:", list)
	c.JSON(200, list)
}
