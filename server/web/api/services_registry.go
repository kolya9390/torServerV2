package api

import (
	"net/http"

	"server/internal/app/contracts"

	"github.com/gin-gonic/gin"
)

const servicesContextKey = "api_handler_deps"

type handlerDeps struct {
	torrents torrentHandlerDeps
	streams  streamHandlerDeps
	settings settingsHandlerDeps
	storage  storageHandlerDeps
	system   systemHandlerDeps
	search   searchHandlerDeps
	media    mediaHandlerDeps
	viewed   viewedHandlerDeps
	playlist playlistHandlerDeps
	play     playHandlerDeps
	cache    cacheHandlerDeps
	upload   uploadHandlerDeps
	tmdb     tmdbHandlerDeps
}

type torrentHandlerDeps struct {
	Queries  contracts.TorrentQueryService
	Commands contracts.TorrentCommandService
	Parser   contracts.TorrentParserService
	Settings contracts.SettingsService
	Modules  contracts.ModulesService
}

type streamTorrentDeps interface {
	contracts.TorrentStreamService
	contracts.TorrentStreamActions
}

type streamHandlerDeps struct {
	Torrents streamTorrentDeps
	Parser   streamParserDeps
	Streams  contracts.StreamOrchestratorService
	Helpers  contracts.StreamHelperService
	Playback contracts.PlaybackService
	Viewed   contracts.ViewedService
}

type streamParserDeps struct {
	Parser  contracts.TorrentParserService
	Helpers contracts.StreamHelperService
}

type settingsHandlerDeps struct {
	Settings contracts.SettingsService
	Modules  contracts.ModulesService
}

type storageHandlerDeps struct {
	Settings contracts.SettingsService
}

type systemHandlerDeps struct {
	Settings contracts.SettingsService
	System   contracts.SystemService
}

type searchHandlerDeps struct {
	Search contracts.SearchService
}

type mediaHandlerDeps struct {
	Media contracts.MediaService
}

type viewedHandlerDeps struct {
	Viewed contracts.ViewedService
}

type playlistHandlerDeps struct {
	Torrents contracts.TorrentPlaylistService
	Playback contracts.PlaybackService
	Viewed   contracts.ViewedService
}

type playHandlerDeps struct {
	Torrents contracts.TorrentPlayService
	Playback contracts.PlaybackService
}

type cacheHandlerDeps struct {
	Torrents contracts.TorrentStorage
}

type uploadHandlerDeps struct {
	Queries  contracts.TorrentLookup
	Commands contracts.TorrentCommandService
	Parser   contracts.TorrentParserService
	Settings contracts.SettingsService
}

type tmdbHandlerDeps struct {
	Settings contracts.SettingsService
}

func buildServicesMiddleware(s *contracts.APIServices) (gin.HandlerFunc, error) {
	if err := validateAPIServices(s); err != nil {
		return nil, err
	}

	return servicesMiddlewareForDeps(newHandlerDeps(s)), nil
}

func servicesMiddleware(s *contracts.APIServices) gin.HandlerFunc {
	middleware, err := buildServicesMiddleware(s)
	if err != nil {
		return func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "api services are not configured: " + err.Error()})
		}
	}

	return middleware
}

func servicesMiddlewareForDeps(deps *handlerDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(servicesContextKey, deps)
		c.Next()
	}
}

func newHandlerDeps(s *contracts.APIServices) *handlerDeps {
	return &handlerDeps{
		torrents: torrentHandlerDeps{
			Queries:  s.Torrents,
			Commands: s.Torrents,
			Parser:   s.Streams,
			Settings: s.Settings,
			Modules:  s.Modules,
		},
		streams: streamHandlerDeps{
			Torrents: s.Torrents,
			Parser: streamParserDeps{
				Parser:  s.Streams,
				Helpers: s.Streams,
			},
			Streams:  s.Streams,
			Helpers:  s.Streams,
			Playback: s.Playback,
			Viewed:   s.Viewed,
		},
		settings: settingsHandlerDeps{
			Settings: s.Settings,
			Modules:  s.Modules,
		},
		storage: storageHandlerDeps{
			Settings: s.Settings,
		},
		system: systemHandlerDeps{
			Settings: s.Settings,
			System:   s.System,
		},
		search: searchHandlerDeps{
			Search: s.Search,
		},
		media: mediaHandlerDeps{
			Media: s.Media,
		},
		viewed: viewedHandlerDeps{
			Viewed: s.Viewed,
		},
		playlist: playlistHandlerDeps{
			Torrents: s.Torrents,
			Playback: s.Playback,
			Viewed:   s.Viewed,
		},
		play: playHandlerDeps{
			Torrents: s.Torrents,
			Playback: s.Playback,
		},
		cache: cacheHandlerDeps{
			Torrents: s.Torrents,
		},
		upload: uploadHandlerDeps{
			Queries:  s.Torrents,
			Commands: s.Torrents,
			Parser:   s.Streams,
			Settings: s.Settings,
		},
		tmdb: tmdbHandlerDeps{
			Settings: s.Settings,
		},
	}
}

func depsFromContext(c *gin.Context) (*handlerDeps, bool) {
	if c == nil {
		return nil, false
	}

	if value, ok := c.Get(servicesContextKey); ok {
		if deps, ok := value.(*handlerDeps); ok && deps != nil {
			return deps, true
		}
	}

	abortAPIError(c, http.StatusInternalServerError, newInternalError("api services are not configured", nil))

	return nil, false
}

func torrentDepsFromContext(c *gin.Context) (torrentHandlerDeps, bool) {
	deps, ok := depsFromContext(c)
	if !ok {
		return torrentHandlerDeps{}, false
	}

	return deps.torrents, true
}

func streamDepsFromContext(c *gin.Context) (streamHandlerDeps, bool) {
	deps, ok := depsFromContext(c)
	if !ok {
		return streamHandlerDeps{}, false
	}

	return deps.streams, true
}

func settingsDepsFromContext(c *gin.Context) (settingsHandlerDeps, bool) {
	deps, ok := depsFromContext(c)
	if !ok {
		return settingsHandlerDeps{}, false
	}

	return deps.settings, true
}

func storageDepsFromContext(c *gin.Context) (storageHandlerDeps, bool) {
	deps, ok := depsFromContext(c)
	if !ok {
		return storageHandlerDeps{}, false
	}

	return deps.storage, true
}

func systemDepsFromContext(c *gin.Context) (systemHandlerDeps, bool) {
	deps, ok := depsFromContext(c)
	if !ok {
		return systemHandlerDeps{}, false
	}

	return deps.system, true
}

func searchDepsFromContext(c *gin.Context) (searchHandlerDeps, bool) {
	deps, ok := depsFromContext(c)
	if !ok {
		return searchHandlerDeps{}, false
	}

	return deps.search, true
}

func mediaDepsFromContext(c *gin.Context) (mediaHandlerDeps, bool) {
	deps, ok := depsFromContext(c)
	if !ok {
		return mediaHandlerDeps{}, false
	}

	return deps.media, true
}

func viewedDepsFromContext(c *gin.Context) (viewedHandlerDeps, bool) {
	deps, ok := depsFromContext(c)
	if !ok {
		return viewedHandlerDeps{}, false
	}

	return deps.viewed, true
}

func playlistDepsFromContext(c *gin.Context) (playlistHandlerDeps, bool) {
	deps, ok := depsFromContext(c)
	if !ok {
		return playlistHandlerDeps{}, false
	}

	return deps.playlist, true
}

func playDepsFromContext(c *gin.Context) (playHandlerDeps, bool) {
	deps, ok := depsFromContext(c)
	if !ok {
		return playHandlerDeps{}, false
	}

	return deps.play, true
}

func cacheDepsFromContext(c *gin.Context) (cacheHandlerDeps, bool) {
	deps, ok := depsFromContext(c)
	if !ok {
		return cacheHandlerDeps{}, false
	}

	return deps.cache, true
}

func uploadDepsFromContext(c *gin.Context) (uploadHandlerDeps, bool) {
	deps, ok := depsFromContext(c)
	if !ok {
		return uploadHandlerDeps{}, false
	}

	return deps.upload, true
}

func tmdbDepsFromContext(c *gin.Context) (tmdbHandlerDeps, bool) {
	deps, ok := depsFromContext(c)
	if !ok {
		return tmdbHandlerDeps{}, false
	}

	return deps.tmdb, true
}
