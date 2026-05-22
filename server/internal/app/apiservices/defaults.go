package apiservices

import (
	"fmt"
	"strings"

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
func NewDefault() (*contracts.APIServices, error) {
	return NewDefaultWithDeps(DefaultDeps{})
}

func NewDefaultWithDeps(deps DefaultDeps) (*contracts.APIServices, error) {
	if err := validateDefaultDeps(deps); err != nil {
		return nil, err
	}

	return &contracts.APIServices{
		Torrents: torrentService{
			backend:        deps.TorrentBackend,
			runtimeSignals: deps.RuntimeSignals,
		},
		Settings: settingsService{
			provider:          deps.SettingsProvider,
			runtimeController: deps.RuntimeController,
			runtimeState:      deps.RuntimeState,
		},
		Viewed: viewedService{
			setViewed:    deps.SetViewed,
			removeViewed: deps.RemoveViewed,
			listViewed:   deps.ListViewed,
		},
		System: systemService{runtimeController: deps.RuntimeController},
		Search: searchService{provider: deps.SettingsProvider},
		Media:  mediaService{runtimeState: deps.RuntimeState},
		Modules: modulesService{
			provider:     deps.SettingsProvider,
			argsProvider: deps.ArgsProvider,
		},
		Streams:  streamService{},
		Playback: playbackService{},
	}, nil
}

func NewDefaultForTests(deps DefaultDeps) (*contracts.APIServices, error) {
	return NewDefaultWithDeps(resolveDefaultDepsForTests(deps))
}

func resolveDefaultDepsForTests(deps DefaultDeps) DefaultDeps {
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

func validateDefaultDeps(deps DefaultDeps) error {
	missing := make([]string, 0)
	required := []struct {
		name    string
		missing bool
	}{
		{name: "TorrentBackend", missing: deps.TorrentBackend == nil},
		{name: "SettingsProvider", missing: deps.SettingsProvider == nil},
		{name: "RuntimeSignals", missing: deps.RuntimeSignals == nil},
		{name: "RuntimeController", missing: deps.RuntimeController == nil},
		{name: "RuntimeState", missing: deps.RuntimeState == nil},
		{name: "ArgsProvider", missing: deps.ArgsProvider == nil},
		{name: "SetViewed", missing: deps.SetViewed == nil},
		{name: "RemoveViewed", missing: deps.RemoveViewed == nil},
		{name: "ListViewed", missing: deps.ListViewed == nil},
	}

	for _, dep := range required {
		if dep.missing {
			missing = append(missing, dep.name)
		}
	}

	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf("apiservices default deps missing: %s", strings.Join(missing, ", "))
}
