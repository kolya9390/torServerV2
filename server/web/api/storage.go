package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetStorageSettings godoc
// @Summary Get storage configuration settings
// @Description Retrieves the current storage preferences for settings and viewed history
// @Tags API
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]interface{} "Storage preferences"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /storage/settings [get].
func GetStorageSettings(c *gin.Context) {
	deps, ok := storageDepsFromContext(c)
	if !ok {
		return
	}

	writeStoragePreferencesResponse(c, deps.Settings.GetStoragePreferences())
}

// UpdateStorageSettings godoc
// @Summary Update storage configuration settings
// @Description Updates the storage preferences for settings and viewed history. Requires application restart for changes to take effect.
// @Tags API
// @Accept json,x-www-form-urlencoded
// @Produce json
// @Security ApiKeyAuth
// @Param request body map[string]interface{} true "Storage preferences to update"
// @Param settings formData string false "Settings storage type" Enums(json,bbolt)
// @Param viewed formData string false "Viewed history storage type" Enums(json,bbolt)
// @Success 200 {object} map[string]string "Update successful"
// @Failure 400 {object} map[string]string "Invalid input data"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 403 {object} map[string]string "Read-only mode"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /storage/settings [post].
func UpdateStorageSettings(c *gin.Context) {
	deps, ok := storageDepsFromContext(c)
	if !ok {
		return
	}

	if deps.Settings.ReadOnly() {
		abortAPIError(c, http.StatusForbidden, newValidationError("mode", "read-only mode"))

		return
	}

	prefs, err := bindStoragePreferencesRequest(c)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	if err := deps.Settings.SetStoragePreferences(prefs); err != nil {
		abortAPIError(c, http.StatusInternalServerError, newInternalError("failed to update storage preferences", err))

		return
	}

	writeStorageUpdateResponse(c)
}
