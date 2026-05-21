package api

import (
	"testing"

	gomock "go.uber.org/mock/gomock"

	"server/internal/app/contracts"
	"server/internal/mocks"
	sets "server/settings"
	"server/torr/state"
)

type mockTorrentService = mocks.MockAPITorrentService
type mockSettingsService = mocks.MockAPISettingsService
type mockViewedService = mocks.MockAPIViewedService
type mockSystemService = mocks.MockAPISystemService
type mockSearchService = mocks.MockAPISearchService
type mockMediaService = mocks.MockAPIMediaService
type mockModulesService = mocks.MockAPIModulesService
type mockStreamService = mocks.MockAPIStreamService
type mockPlaybackService = mocks.MockAPIPlaybackService

func newAPIServicesFixture(t *testing.T, overrides *contracts.APIServices) *contracts.APIServices {
	t.Helper()

	ctrl := gomock.NewController(t)
	services := newDefaultAPIServicesFixture(ctrl)

	if overrides == nil {
		return services
	}

	applyServiceOverrides(services, overrides)

	return services
}

func newDefaultAPIServicesFixture(ctrl *gomock.Controller) *contracts.APIServices {
	torrents := mocks.NewMockAPITorrentService(ctrl)
	settings := mocks.NewMockAPISettingsService(ctrl)
	viewed := mocks.NewMockAPIViewedService(ctrl)
	system := mocks.NewMockAPISystemService(ctrl)
	search := mocks.NewMockAPISearchService(ctrl)
	media := mocks.NewMockAPIMediaService(ctrl)
	modules := mocks.NewMockAPIModulesService(ctrl)
	streams := mocks.NewMockAPIStreamService(ctrl)
	playback := mocks.NewMockAPIPlaybackService(ctrl)

	torrents.EXPECT().Add(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, contracts.ErrStreamConnectionTimeout).AnyTimes()
	torrents.EXPECT().Get(gomock.Any()).Return(nil).AnyTimes()
	torrents.EXPECT().Status(gomock.Any()).Return(nil).AnyTimes()
	torrents.EXPECT().StatusByHash(gomock.Any()).Return(nil, false).AnyTimes()
	torrents.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	torrents.EXPECT().SaveToDB(gomock.Any()).AnyTimes()
	torrents.EXPECT().Remove(gomock.Any()).AnyTimes()
	torrents.EXPECT().List().Return([]contracts.TorrentHandle{}).AnyTimes()
	torrents.EXPECT().Statuses().Return([]*state.TorrentStatus{}).AnyTimes()
	torrents.EXPECT().ListHashes().Return([]string{}).AnyTimes()
	torrents.EXPECT().Drop(gomock.Any()).AnyTimes()
	torrents.EXPECT().IsStored(gomock.Any()).Return(false).AnyTimes()
	torrents.EXPECT().DropReadiness(gomock.Any()).Return(contracts.DropReadiness{}).AnyTimes()
	torrents.EXPECT().CacheStateByHash(gomock.Any()).Return(nil, false).AnyTimes()
	torrents.EXPECT().EnqueuePreload(gomock.Any(), gomock.Any()).Return(false).AnyTimes()
	torrents.EXPECT().EnqueueMetadataFinalize(gomock.Any(), gomock.Any(), gomock.Any()).Return(false).AnyTimes()
	torrents.EXPECT().LoadFromDB(gomock.Any()).DoAndReturn(func(tor contracts.TorrentHandle) contracts.TorrentHandle {
		return tor
	}).AnyTimes()

	settings.EXPECT().Current().Return(nil).AnyTimes()
	settings.EXPECT().Set(gomock.Any()).AnyTimes()
	settings.EXPECT().SetDefault().AnyTimes()
	settings.EXPECT().ReadOnly().Return(false).AnyTimes()
	settings.EXPECT().GetStoragePreferences().Return(map[string]any{}).AnyTimes()
	settings.EXPECT().SetStoragePreferences(gomock.Any()).Return(nil).AnyTimes()
	settings.EXPECT().TMDBConfig().Return(sets.TMDBConfig{}, false).AnyTimes()
	settings.EXPECT().BuildPlayURL(gomock.Any(), gomock.Any()).Return("").AnyTimes()
	settings.EXPECT().EnableDLNA().Return(false).AnyTimes()
	settings.EXPECT().EnableDebug().Return(false).AnyTimes()

	viewed.EXPECT().SetViewed(gomock.Any()).AnyTimes()
	viewed.EXPECT().RemoveViewed(gomock.Any()).AnyTimes()
	viewed.EXPECT().ListViewed(gomock.Any()).Return(nil).AnyTimes()

	system.EXPECT().Shutdown().AnyTimes()

	search.EXPECT().EnableTorznabSearch().Return(false).AnyTimes()
	search.EXPECT().TorznabSearch(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	search.EXPECT().TorznabTest(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	media.EXPECT().ProbePlayURL(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	modules.EXPECT().RestartDLNA(gomock.Any()).Return(nil).AnyTimes()
	modules.EXPECT().StopDLNA().AnyTimes()

	streams.EXPECT().ParseLink(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(contracts.TorrentSpec{}, contracts.StreamMeta{}, contracts.ErrStreamInvalidLink).AnyTimes()
	streams.EXPECT().ParseTorrentFile(gomock.Any()).Return(contracts.TorrentSpec{}, contracts.ErrStreamInvalidLink).AnyTimes()
	streams.EXPECT().EnsureTorrent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, contracts.ErrStreamUnauthorized).AnyTimes()
	streams.EXPECT().ParseFileIndex(gomock.Any(), gomock.Any()).Return(0, contracts.ErrStreamFileIndexInvalid).AnyTimes()
	streams.EXPECT().NormalizePlaylistName(gomock.Any(), gomock.Any()).DoAndReturn(func(_ string, fallback string) string {
		return fallback + ".m3u"
	}).AnyTimes()

	playback.EXPECT().BuildAllPlaylist(gomock.Any(), gomock.Any()).Return(contracts.PlaylistPayload{Name: "all.m3u", Body: "#EXTM3U\n"}).AnyTimes()
	playback.EXPECT().BuildPlaylistByHash(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(contracts.PlaylistPayload{}, contracts.ErrPlaylistTorrentNotFound).AnyTimes()
	playback.EXPECT().BuildM3UFromStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("").AnyTimes()
	playback.EXPECT().ResolvePlay(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(contracts.PlayTarget{}, contracts.ErrPlayTorrentNotFound).AnyTimes()

	return &contracts.APIServices{
		Torrents: torrents,
		Settings: settings,
		Viewed:   viewed,
		System:   system,
		Search:   search,
		Media:    media,
		Modules:  modules,
		Streams:  streams,
		Playback: playback,
	}
}

func applyServiceOverrides(services, overrides *contracts.APIServices) {
	if overrides.Torrents != nil {
		services.Torrents = overrides.Torrents
	}

	if overrides.Settings != nil {
		services.Settings = overrides.Settings
	}

	if overrides.Viewed != nil {
		services.Viewed = overrides.Viewed
	}

	if overrides.System != nil {
		services.System = overrides.System
	}

	if overrides.Search != nil {
		services.Search = overrides.Search
	}

	if overrides.Media != nil {
		services.Media = overrides.Media
	}

	if overrides.Modules != nil {
		services.Modules = overrides.Modules
	}

	if overrides.Streams != nil {
		services.Streams = overrides.Streams
	}

	if overrides.Playback != nil {
		services.Playback = overrides.Playback
	}
}
