package api

import (
	"fmt"
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
	svc := getServices()

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
		setViewed(svc, req, c)
	case "rem":
		remViewed(svc, req, c)
	case "list":
		listViewed(svc, req, c)
	default:
		abortAPIError(c, http.StatusBadRequest, newValidationError("action", "must be one of: set, rem, list"))
	}
}

func setViewed(svc *APIServices, req viewedReqJS, c *gin.Context) {
	if svc == nil || svc.Viewed == nil || req.Viewed == nil {
		abortAPIError(c, http.StatusBadRequest, newValidationError("viewed", "is required for action=set"))

		return
	}

	svc.Viewed.SetViewed(req.Viewed)
	c.Status(200)
}

func remViewed(svc *APIServices, req viewedReqJS, c *gin.Context) {
	if svc == nil || svc.Viewed == nil || req.Viewed == nil {
		abortAPIError(c, http.StatusBadRequest, newValidationError("viewed", "is required for action=rem"))

		return
	}

	svc.Viewed.RemoveViewed(req.Viewed)
	c.Status(200)
}

func listViewed(svc *APIServices, req viewedReqJS, c *gin.Context) {
	log.TLogln("listViewed: START")
	log.TLogln("listViewed: svc is nil?", svc == nil)

	if svc != nil {
		log.TLogln("listViewed: svc.Viewed is nil?", svc.Viewed == nil)
	}

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

func callListViewed(viewed ViewedService, hash string) (result []*sets.Viewed) {
	log.TLogln("callListViewed: ENTRY, hash:", hash)
	log.TLogln("callListViewed: viewed is nil?", viewed == nil)
	log.TLogln("callListViewed: viewed type:", fmt.Sprintf("%T", viewed))

	defer func() {
		if r := recover(); r != nil {
			log.TLogln("callListViewed panic:", r)
			log.TLogln("stack:", string(debug.Stack()))

			result = []*sets.Viewed{}
		}
	}()
	log.TLogln("callListViewed: before type switch")

	switch v := viewed.(type) {
	case interface{ ListViewed(string) []*sets.Viewed }:
		log.TLogln("callListViewed: matched interface with ListViewed method")

		result = v.ListViewed(hash)
	default:
		log.TLogln("callListViewed: type doesn't match, using direct call")

		result = viewed.ListViewed(hash)
	}

	log.TLogln("callListViewed: got result:", result)

	return result
}
