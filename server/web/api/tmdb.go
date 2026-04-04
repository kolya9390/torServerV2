package api

import (
	"github.com/gin-gonic/gin"
)

// tmdbSettings godoc
//
//	@Summary		Get TMDB settings
//	@Description	Get TMDB API configuration
//
//	@Tags			API
//
//	@Produce		json
//	@Success		200	{object}	sets.TMDBConfig	"TMDB settings"
//	@Router			/tmdb/settings [get]
func tmdbSettings(c *gin.Context) {
	cfg, ok := getServices().Settings.TMDBConfig()
	if !ok {
		abortAPIError(c, 500, newInternalError("settings not initialized", nil))
		return
	}
	c.JSON(200, cfg)
}
