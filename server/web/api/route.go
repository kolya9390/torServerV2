package api

import (
	sets "server/settings"
	"server/web/auth"

	"github.com/gin-gonic/gin"
)

type requestI struct {
	Action string `json:"action,omitempty"`
}

func SetupRoute(route gin.IRouter) {
	route.GET("/api/version", apiVersion)
	route.GET("/api/v1/version", apiVersion)

	legacy := route.Group("/", legacyDeprecationHeaders())
	registerAPIRoutes(legacy)

	v1 := route.Group("/api/v1")
	registerAPIRoutes(v1)
}

func registerAPIRoutes(route gin.IRouter) {
	authorized := route.Group("/", auth.CheckAuth())

	authorized.GET("/shutdown", shutdown)
	authorized.GET("/shutdown/*reason", shutdown)
	authorized.POST("/shutdown", shutdown)
	authorized.POST("/shutdown/*reason", shutdown)

	authorized.POST("/settings", settings)
	authorized.POST("/torznab/test", torznabTest)

	authorized.POST("/torrents", torrents)

	authorized.POST("/torrent/upload", torrentUpload)

	authorized.POST("/cache", cache)

	route.HEAD("/stream", stream)
	route.GET("/stream", stream)

	route.HEAD("/stream/*fname", stream)
	route.GET("/stream/*fname", stream)

	// Explicit stream API (read-only and command endpoints)
	route.HEAD("/streams/stat", streamStat)
	route.GET("/streams/stat", streamStat)
	route.HEAD("/streams/m3u", streamM3U)
	route.GET("/streams/m3u", streamM3U)
	route.HEAD("/streams/play", streamPlay)
	route.GET("/streams/play", streamPlay)
	authorized.POST("/streams/save", streamSave)

	route.HEAD("/play/:hash/:id", play)
	route.GET("/play/:hash/:id", play)

	authorized.POST("/viewed", viewed)

	authorized.GET("/playlistall/all.m3u", allPlayList)

	route.GET("/playlist", playList)
	route.GET("/playlist/*fname", playList)

	authorized.GET("/download/:size", download)

	// Torznab search only (Rutor removed)
	if sets.SearchWA {
		route.GET("/torznab/search/*query", torznabSearch)
	} else {
		authorized.GET("/torznab/search/*query", torznabSearch)
	}

	// Add storage settings endpoints
	authorized.GET("/storage/settings", GetStorageSettings)
	authorized.POST("/storage/settings", UpdateStorageSettings)

	// Add TMDB settings endpoint
	authorized.GET("/tmdb/settings", tmdbSettings)

	authorized.GET("/ffp/:hash/:id", ffp)
}
