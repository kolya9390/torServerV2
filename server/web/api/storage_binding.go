package api

import (
	"strings"

	"github.com/gin-gonic/gin"
)

func bindStoragePreferencesRequest(c *gin.Context) (map[string]any, error) {
	prefs, err := parseStoragePreferences(c)
	if err != nil {
		return nil, err
	}

	if err := validateStoragePreferences(prefs); err != nil {
		return nil, err
	}

	return prefs, nil
}

func parseStoragePreferences(c *gin.Context) (map[string]any, error) {
	if strings.HasPrefix(c.GetHeader("Content-Type"), "application/x-www-form-urlencoded") {
		prefs := make(map[string]any)
		if settings := c.PostForm("settings"); settings != "" {
			prefs["settings"] = settings
		}

		if viewed := c.PostForm("viewed"); viewed != "" {
			prefs["viewed"] = viewed
		}

		return prefs, nil
	}

	var prefs map[string]any
	if err := c.ShouldBindJSON(&prefs); err != nil {
		return nil, newValidationError("request", "invalid input data")
	}

	return prefs, nil
}

func validateStoragePreferences(prefs map[string]any) error {
	if settingsPref, ok := prefs["settings"].(string); ok && settingsPref != "" {
		if settingsPref != "json" && settingsPref != "bbolt" {
			return newValidationError("settings", "must be json or bbolt")
		}
	}

	if viewedPref, ok := prefs["viewed"].(string); ok && viewedPref != "" {
		if viewedPref != "json" && viewedPref != "bbolt" {
			return newValidationError("viewed", "must be json or bbolt")
		}
	}

	if len(prefs) == 0 {
		return newValidationError("request", "no preferences provided")
	}

	return nil
}
