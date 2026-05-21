package apiservices

import (
	"server/internal/app/contracts"
	sets "server/settings"
	"server/torr"
)

type DefaultDeps struct {
	TorrentBackend    torr.TorrentService
	SettingsProvider  sets.SettingsProvider
	RuntimeSignals    torr.RuntimeSignals
	RuntimeController torr.RuntimeController
	RuntimeState      func() sets.RuntimeState
	ArgsProvider      sets.ArgsProvider
	SetViewed         func(*sets.Viewed)
	RemoveViewed      func(*sets.Viewed)
	ListViewed        func(string) []*sets.Viewed
}

type torrentService struct {
	backend        torr.TorrentService
	runtimeSignals torr.RuntimeSignals
}
type settingsService struct {
	provider          sets.SettingsProvider
	runtimeController torr.RuntimeController
	runtimeState      func() sets.RuntimeState
}
type viewedService struct {
	setViewed    func(*sets.Viewed)
	removeViewed func(*sets.Viewed)
	listViewed   func(string) []*sets.Viewed
}
type systemService struct {
	runtimeController torr.RuntimeController
}
type searchService struct {
	provider sets.SettingsProvider
}
type mediaService struct {
	runtimeState func() sets.RuntimeState
}
type modulesService struct {
	provider     sets.SettingsProvider
	argsProvider sets.ArgsProvider
}
type streamService struct{}
type playbackService struct{}

// NewDefault constructs the default API application services using runtime adapters.
func NewDefault() *contracts.APIServices {
	return NewDefaultWithDeps(DefaultDeps{})
}

func NewDefaultWithDeps(deps DefaultDeps) *contracts.APIServices {
	resolved := resolveDefaultDeps(deps)

	return &contracts.APIServices{
		Torrents: torrentService{
			backend:        resolved.TorrentBackend,
			runtimeSignals: resolved.RuntimeSignals,
		},
		Settings: settingsService{
			provider:          resolved.SettingsProvider,
			runtimeController: resolved.RuntimeController,
			runtimeState:      resolved.RuntimeState,
		},
		Viewed: viewedService{
			setViewed:    resolved.SetViewed,
			removeViewed: resolved.RemoveViewed,
			listViewed:   resolved.ListViewed,
		},
		System: systemService{runtimeController: resolved.RuntimeController},
		Search: searchService{provider: resolved.SettingsProvider},
		Media:  mediaService{runtimeState: resolved.RuntimeState},
		Modules: modulesService{
			provider:     resolved.SettingsProvider,
			argsProvider: resolved.ArgsProvider,
		},
		Streams:  streamService{},
		Playback: playbackService{},
	}
}

func resolveDefaultDeps(deps DefaultDeps) DefaultDeps {
	if deps.TorrentBackend == nil {
		deps.TorrentBackend = torr.NewNoopTorrentService()
	}

	if deps.SettingsProvider == nil {
		deps.SettingsProvider = sets.NewNoopSettingsProvider()
	}

	if deps.RuntimeSignals == nil {
		deps.RuntimeSignals = torr.NewNoopRuntimeSignals()
	}

	if deps.RuntimeController == nil {
		deps.RuntimeController = torr.NewNoopRuntimeController()
	}

	if deps.RuntimeState == nil {
		deps.RuntimeState = func() sets.RuntimeState { return sets.RuntimeState{} }
	}

	if deps.ArgsProvider == nil {
		deps.ArgsProvider = sets.NewNoopArgsProvider()
	}

	if deps.SetViewed == nil {
		deps.SetViewed = sets.SetViewed
	}

	if deps.RemoveViewed == nil {
		deps.RemoveViewed = sets.RemViewed
	}

	if deps.ListViewed == nil {
		deps.ListViewed = sets.ListViewed
	}

	return deps
}
