package api

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	sets "server/settings"
)

// Action: get, set, def.
type setsReqJS struct {
	requestI
	Sets *sets.BTSets `json:"sets,omitempty"`
}

// settings godoc
//
//	@Summary		Get / Set server settings
//	@Description	Allow to get or set server settings.
//
//	@Tags			API
//
//	@Param			request	body	setsReqJS	true	"Settings request. Available params for action: get, set, def"
//
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	sets.BTSets	"Settings JSON or nothing. Depends on what action has been asked."
//	@Router			/settings [post]
func settings(c *gin.Context) {
	svc := getServices()

	var req setsReqJS

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
		current := svc.Settings.Current()

		etag := generateSettingsETag(current)
		if match := c.GetHeader("If-None-Match"); match == etag {
			c.Status(http.StatusNotModified)

			return
		}

		c.Header("ETag", etag)
		c.Header("Cache-Control", "private, max-age=5")
		c.Header("Content-Type", "application/json")

		data, _ := json.Marshal(current)
		c.Data(200, "application/json", data)

		return
	case "set":
		if req.Sets == nil {
			abortAPIError(c, http.StatusBadRequest, newValidationError("sets", "is required for action=set"))

			return
		}

		svc.Settings.Set(req.Sets)

		if err := svc.Modules.RestartDLNA(req.Sets.EnableDLNA); err != nil {
			abortAPIError(c, http.StatusInternalServerError, newInternalError("dlna start failed", err))

			return
		}

		c.Status(200)

		return
	case "def":
		svc.Settings.SetDefault()
		svc.Modules.StopDLNA()
		c.Status(200)

		return
	default:
		abortAPIError(c, http.StatusBadRequest, newValidationError("action", "must be one of: get, set, def"))
	}
}
