package api

import (
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

func servicesMiddleware(s *contracts.APIServices) gin.HandlerFunc {
	if err := validateAPIServices(s); err != nil {
		panic("api services are not configured: " + err.Error())
	}

	deps := newHandlerDeps(s)

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

func depsFromContext(c *gin.Context) *handlerDeps {
	if c == nil {
		panic("api services are not configured: nil gin context")
	}

	if value, ok := c.Get(servicesContextKey); ok {
		if deps, ok := value.(*handlerDeps); ok && deps != nil {
			return deps
		}
	}

	panic("api services are not configured in gin context")
}

func torrentDepsFromContext(c *gin.Context) torrentHandlerDeps {
	return depsFromContext(c).torrents
}

func streamDepsFromContext(c *gin.Context) streamHandlerDeps {
	return depsFromContext(c).streams
}

func settingsDepsFromContext(c *gin.Context) settingsHandlerDeps {
	return depsFromContext(c).settings
}

func storageDepsFromContext(c *gin.Context) storageHandlerDeps {
	return depsFromContext(c).storage
}

func systemDepsFromContext(c *gin.Context) systemHandlerDeps {
	return depsFromContext(c).system
}

func searchDepsFromContext(c *gin.Context) searchHandlerDeps {
	return depsFromContext(c).search
}

func mediaDepsFromContext(c *gin.Context) mediaHandlerDeps {
	return depsFromContext(c).media
}

func viewedDepsFromContext(c *gin.Context) viewedHandlerDeps {
	return depsFromContext(c).viewed
}

func playlistDepsFromContext(c *gin.Context) playlistHandlerDeps {
	return depsFromContext(c).playlist
}

func playDepsFromContext(c *gin.Context) playHandlerDeps {
	return depsFromContext(c).play
}

func cacheDepsFromContext(c *gin.Context) cacheHandlerDeps {
	return depsFromContext(c).cache
}

func uploadDepsFromContext(c *gin.Context) uploadHandlerDeps {
	return depsFromContext(c).upload
}

func tmdbDepsFromContext(c *gin.Context) tmdbHandlerDeps {
	return depsFromContext(c).tmdb
}
