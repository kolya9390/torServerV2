package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

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
//	@Success		200	{object}	contracts.Settings	"Settings JSON or nothing. Depends on what action has been asked."
//	@Router			/settings [post]
func settings(c *gin.Context) {
	req, err := bindSettingsRequest(c)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	deps := settingsDepsFromContext(c)

	switch req.Action {
	case "get":
		writeSettingsResponse(c, deps.Settings.Current())

		return
	case "set":
		// Block EnableDebug changes via API — only config.yml controls debug mode.
		// This prevents runtime toggling and ensures debug is set at startup.
		req.Sets.EnableDebug = false

		deps.Settings.Set(req.Sets)

		if err := deps.Modules.RestartDLNA(req.Sets.EnableDLNA); err != nil {
			abortAPIError(c, http.StatusInternalServerError, newInternalError("dlna start failed", err))

			return
		}

		c.Status(200)

		return
	case "def":
		deps.Settings.SetDefault()
		deps.Modules.StopDLNA()
		c.Status(200)

		return
	default:
		abortAPIError(c, http.StatusBadRequest, newValidationError("action", "must be one of: get, set, def"))
	}
}
