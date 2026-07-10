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

	deps, ok := settingsDepsFromContext(c)
	if !ok {
		return
	}

	switch req.Action {
	case "get":
		writeSettingsResponse(c, deps.Settings.Current())

		return
	case "set":
		current := deps.Settings.Current()
		merged, err := mergeSettingsPatch(current, req.Sets, req.SetsRaw)
		if err != nil {
			abortAPIError(c, http.StatusBadRequest, err)

			return
		}

		// Block EnableDebug changes via API: debug remains a startup/config.yml decision.
		enableDebug := false
		if current != nil {
			enableDebug = current.EnableDebug
		}
		merged.EnableDebug = enableDebug

		deps.Settings.Set(merged)

		if err := deps.Modules.RestartDLNA(merged.EnableDLNA); err != nil {
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
