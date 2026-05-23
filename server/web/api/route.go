package api

import (
	"errors"
	"fmt"
	"strings"

	"server/internal/app/contracts"
	sets "server/settings"
	authapi "server/web/api/auth"
	"server/web/auth"

	"github.com/gin-gonic/gin"
)

type requestI struct {
	Action string `json:"action,omitempty"`
}

func SetupRouteWithServices(route gin.IRouter, runtimeState func() sets.RuntimeState, services *contracts.APIServices) error {
	middleware, err := buildServicesMiddleware(services)
	if err != nil {
		return fmt.Errorf("api services are not configured: %w", err)
	}

	route.GET("/api/version", apiVersion)
	route.GET("/api/v1/version", apiVersion)

	legacy := route.Group("/", legacyDeprecationHeaders())
	registerAPIRoutes(legacy, runtimeState, middleware)

	v1 := route.Group("/api/v1")
	registerAPIRoutes(v1, runtimeState, middleware)

	return nil
}

func registerAPIRoutes(route gin.IRouter, runtimeState func() sets.RuntimeState, middleware gin.HandlerFunc) {
	route.Use(middleware)
	authorized := route.Group("/", auth.CheckAuth())

	if runtimeState == nil {
		runtimeState = func() sets.RuntimeState { return sets.RuntimeState{} }
	}

	authCfg := runtimeState().AuthConfig()

	registerManagementRoutes(authorized)
	registerTorrentRoutes(authorized)
	registerPreferredStreamRoutes(route, authorized)
	registerCompatibilityPlaybackRoutes(route, authorized)
	registerSearchRoutes(route, authorized, authCfg)
	registerSettingsRoutes(authorized)
	authapi.RegisterAuthRoutes(authorized)
}

func registerManagementRoutes(authorized gin.IRouter) {
	authorized.GET("/shutdown", shutdown)
	authorized.GET("/shutdown/*reason", shutdown)
	authorized.POST("/shutdown", shutdown)
	authorized.POST("/shutdown/*reason", shutdown)
}

func registerSettingsRoutes(authorized gin.IRouter) {
	authorized.POST("/settings", settings)
	authorized.POST("/torznab/test", torznabTest)

	authorized.GET("/storage/settings", GetStorageSettings)
	authorized.POST("/storage/settings", UpdateStorageSettings)

	authorized.GET("/tmdb/settings", tmdbSettings)
}

func registerTorrentRoutes(authorized gin.IRouter) {
	authorized.POST("/torrents", torrents)
	authorized.POST("/torrent/upload", torrentUpload)
	authorized.POST("/cache", cache)
	authorized.POST("/viewed", viewed)
}

func registerPreferredStreamRoutes(route gin.IRouter, authorized gin.IRouter) {
	// Preferred stream API: explicit read and command endpoints used by new integrations.
	route.HEAD("/streams/stat", streamStat)
	route.GET("/streams/stat", streamStat)
	route.HEAD("/streams/m3u", streamM3U)
	route.GET("/streams/m3u", streamM3U)
	route.HEAD("/streams/play", streamPlay)
	route.GET("/streams/play", streamPlay)
	authorized.POST("/streams/save", streamSave)
}

func registerCompatibilityPlaybackRoutes(route gin.IRouter, authorized gin.IRouter) {
	// Compatibility stream/playback routes for legacy clients and generated URLs.
	route.HEAD("/stream", stream)
	route.GET("/stream", stream)
	route.HEAD("/stream/*fname", stream)
	route.GET("/stream/*fname", stream)

	route.HEAD("/play/:hash/:id", play)
	route.GET("/play/:hash/:id", play)

	authorized.GET("/playlistall/all.m3u", allPlayList)

	route.GET("/playlist", playList)
	route.GET("/playlist/*fname", playList)

	authorized.GET("/download/:size", download)
	authorized.GET("/ffp/:hash/:id", ffp)
}

func registerSearchRoutes(route gin.IRouter, authorized gin.IRouter, authCfg sets.RuntimeAuthConfig) {
	// Torznab search wildcard path is preserved for compatibility with Torznab clients.
	if authCfg.SearchWA {
		route.GET("/torznab/search/*query", torznabSearch)
	} else {
		authorized.GET("/torznab/search/*query", torznabSearch)
	}
}

func validateAPIServices(services *contracts.APIServices) error {
	if services == nil {
		return errors.New("services is nil")
	}

	missing := make([]string, 0)
	if services.Torrents == nil {
		missing = append(missing, "Torrents")
	}

	if services.Settings == nil {
		missing = append(missing, "Settings")
	}

	if services.Viewed == nil {
		missing = append(missing, "Viewed")
	}

	if services.System == nil {
		missing = append(missing, "System")
	}

	if services.Search == nil {
		missing = append(missing, "Search")
	}

	if services.Media == nil {
		missing = append(missing, "Media")
	}

	if services.Modules == nil {
		missing = append(missing, "Modules")
	}

	if services.Streams == nil {
		missing = append(missing, "Streams")
	}

	if services.Playback == nil {
		missing = append(missing, "Playback")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing %s", strings.Join(missing, ", "))
	}

	return nil
}
