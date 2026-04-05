package api

import (
	"errors"
	"sync"

	"github.com/anacrolix/torrent"
	goffprobe "gopkg.in/vansante/go-ffprobe.v2"

	sets "server/settings"
	"server/torr"
	"server/torr/state"
	"server/torznab"
)

// TorrentService defines application-level torrent use-cases consumed by HTTP handlers.
type TorrentService interface {
	Add(spec *torrent.TorrentSpec, title, poster, data, category string) (*torr.Torrent, error)
	Get(hash string) *torr.Torrent
	Set(hash, title, poster, category, data string) *torr.Torrent
	SaveToDB(tor *torr.Torrent)
	Remove(hash string)
	List() []*torr.Torrent
	Drop(hash string)
	EnqueuePreload(tor *torr.Torrent, index int) bool
	EnqueueMetadataFinalize(tor *torr.Torrent, spec *torrent.TorrentSpec, saveToDB bool) bool
	LoadFromDB(tor *torr.Torrent) *torr.Torrent
}

// SettingsService defines settings use-cases for API handlers.
type SettingsService interface {
	Current() *sets.BTSets
	Set(*sets.BTSets)
	SetDefault()
	ReadOnly() bool
	GetStoragePreferences() map[string]any
	SetStoragePreferences(map[string]any) error
	TMDBConfig() (sets.TMDBConfig, bool)
	BuildPlayURL(hash, fileID string) string
	EnableDLNA() bool
	EnableDebug() bool
}

// ViewedService defines viewed-history operations consumed by handlers.
type ViewedService interface {
	SetViewed(v *sets.Viewed)
	RemoveViewed(v *sets.Viewed)
	ListViewed(hash string) []*sets.Viewed
}

// SystemService defines process-level operations used by API handlers.
type SystemService interface {
	Shutdown()
}

// SearchService defines external search integrations.
type SearchService interface {
	EnableTorznabSearch() bool
	TorznabSearch(query string, index int) []*torznab.TorrentDetails
	TorznabTest(host, key string) error
}

// MediaService defines media metadata operations used by API handlers.
type MediaService interface {
	ProbePlayURL(hash, fileID string) (*goffprobe.ProbeData, error)
}

// ModulesService defines peripheral module operations used by API handlers.
type ModulesService interface {
	RestartDLNA(enable bool) error
	StopDLNA()
}

// StreamMeta carries optional metadata for stream-oriented operations.
type StreamMeta struct {
	Title    string
	Poster   string
	Category string
	Data     string
}

var (
	// ErrStreamLinkEmpty indicates missing stream link parameter.
	ErrStreamLinkEmpty = errors.New("stream link is empty")
	// ErrStreamInvalidTorrsHash indicates invalid torrs hash payload.
	ErrStreamInvalidTorrsHash = errors.New("stream torrs hash is invalid")
	// ErrStreamInvalidLink indicates malformed magnet/hash/link payload.
	ErrStreamInvalidLink = errors.New("stream link is invalid")
	// ErrStreamUnauthorized indicates that stream operation requires auth.
	ErrStreamUnauthorized = errors.New("stream authorization required")
	// ErrStreamConnectionTimeout indicates torrent metadata/connect timeout.
	ErrStreamConnectionTimeout = errors.New("stream torrent connection timeout")
	// ErrStreamFileIndexInvalid indicates invalid stream file index.
	ErrStreamFileIndexInvalid = errors.New("stream file index is invalid")

	// ErrPlaylistHashRequired indicates missing torrent hash for playlist operations.
	ErrPlaylistHashRequired = errors.New("playlist hash is required")
	// ErrPlaylistTorrentNotFound indicates missing torrent for requested playlist.
	ErrPlaylistTorrentNotFound = errors.New("playlist torrent not found")
	// ErrPlaylistLoadFailed indicates failure to load torrent metadata from storage.
	ErrPlaylistLoadFailed = errors.New("playlist load from db failed")

	// ErrPlayPathRequired indicates missing path params for play endpoint.
	ErrPlayPathRequired = errors.New("play hash and id are required")
	// ErrPlayHashInvalid indicates malformed play hash link/infohash.
	ErrPlayHashInvalid = errors.New("play hash is invalid")
	// ErrPlayUnauthorized indicates play operation requires authorization.
	ErrPlayUnauthorized = errors.New("play authorization required")
	// ErrPlayTorrentNotFound indicates requested torrent is not active.
	ErrPlayTorrentNotFound = errors.New("play torrent not found")
	// ErrPlayLoadFailed indicates failure to restore torrent from persistent storage.
	ErrPlayLoadFailed = errors.New("play load from db failed")
	// ErrPlayTimeout indicates torrent metadata timeout before play.
	ErrPlayTimeout = errors.New("play torrent connection timeout")
	// ErrPlayFileIndexInvalid indicates invalid file index for play operation.
	ErrPlayFileIndexInvalid = errors.New("play file index is invalid")
)

// StreamService defines stream orchestration helpers used by transport handlers.
type StreamService interface {
	ParseLink(link, title, poster, category string) (*torrent.TorrentSpec, StreamMeta, error)
	EnsureTorrent(torrents TorrentService, spec *torrent.TorrentSpec, meta StreamMeta, allowCreate bool) (*torr.Torrent, error)
	ParseFileIndex(index string, fileCount int) (int, error)
	NormalizePlaylistName(rawName, fallback string) string
}

// PlaylistPayload contains generated M3U payload details.
type PlaylistPayload struct {
	Name string
	Hash string
	Body string
}

// PlayTarget contains resolved torrent/file index for play endpoint.
type PlayTarget struct {
	Torrent   *torr.Torrent
	FileIndex int
}

// PlaybackService contains playlist and play orchestration logic.
type PlaybackService interface {
	BuildAllPlaylist(host string, torrents TorrentService) PlaylistPayload
	BuildPlaylistByHash(hash, requestedName string, fromLast bool, host string, torrents TorrentService, viewed ViewedService) (PlaylistPayload, error)
	BuildM3UFromStatus(tor *state.TorrentStatus, host string, fromLast bool, viewed ViewedService) string
	ResolvePlay(hash, index string, unauthorized bool, torrents TorrentService) (PlayTarget, error)
}

// APIServices aggregates dependencies used by transport handlers.
type APIServices struct {
	Torrents TorrentService
	Settings SettingsService
	Viewed   ViewedService
	System   SystemService
	Search   SearchService
	Media    MediaService
	Modules  ModulesService
	Streams  StreamService
	Playback PlaybackService
}

type noopTorrentService struct{}
type noopSettingsService struct{}
type noopViewedService struct{}
type noopSystemService struct{}
type noopSearchService struct{}
type noopMediaService struct{}
type noopModulesService struct{}
type noopStreamService struct{}
type noopPlaybackService struct{}

var (
	apiServicesMu sync.RWMutex
	apiServices   *APIServices
)

func (noopTorrentService) Add(spec *torrent.TorrentSpec, title, poster, data, category string) (*torr.Torrent, error) {
	return nil, nil
}
func (noopTorrentService) Get(hash string) *torr.Torrent { return nil }
func (noopTorrentService) Set(hash, title, poster, category, data string) *torr.Torrent {
	return nil
}
func (noopTorrentService) SaveToDB(tor *torr.Torrent) {}
func (noopTorrentService) Remove(hash string)         {}
func (noopTorrentService) List() []*torr.Torrent      { return []*torr.Torrent{} }
func (noopTorrentService) Drop(hash string)           {}
func (noopTorrentService) EnqueuePreload(tor *torr.Torrent, index int) bool {
	return false
}
func (noopTorrentService) EnqueueMetadataFinalize(tor *torr.Torrent, spec *torrent.TorrentSpec, saveToDB bool) bool {
	return false
}
func (noopTorrentService) LoadFromDB(tor *torr.Torrent) *torr.Torrent { return tor }

func (noopSettingsService) Current() *sets.BTSets { return nil }
func (noopSettingsService) Set(*sets.BTSets)      {}
func (noopSettingsService) SetDefault()           {}
func (noopSettingsService) ReadOnly() bool        { return false }
func (noopSettingsService) GetStoragePreferences() map[string]any {
	return map[string]any{}
}
func (noopSettingsService) SetStoragePreferences(map[string]any) error { return nil }
func (noopSettingsService) TMDBConfig() (sets.TMDBConfig, bool)        { return sets.TMDBConfig{}, false }
func (noopSettingsService) BuildPlayURL(hash, fileID string) string    { return "" }
func (noopSettingsService) EnableDLNA() bool                           { return false }
func (noopSettingsService) EnableDebug() bool                          { return false }
func (noopViewedService) SetViewed(v *sets.Viewed)                     {}
func (noopViewedService) RemoveViewed(v *sets.Viewed)                  {}
func (noopViewedService) ListViewed(hash string) []*sets.Viewed        { return []*sets.Viewed{} }
func (noopSystemService) Shutdown()                                    {}
func (noopSearchService) EnableTorznabSearch() bool                    { return false }
func (noopSearchService) TorznabSearch(query string, index int) []*torznab.TorrentDetails {
	return []*torznab.TorrentDetails{}
}
func (noopSearchService) TorznabTest(host, key string) error { return nil }
func (noopMediaService) ProbePlayURL(hash, fileID string) (*goffprobe.ProbeData, error) {
	return nil, nil
}
func (noopModulesService) RestartDLNA(enable bool) error { return nil }
func (noopModulesService) StopDLNA()                     {}
func (noopStreamService) ParseLink(link, title, poster, category string) (*torrent.TorrentSpec, StreamMeta, error) {
	return nil, StreamMeta{}, ErrStreamInvalidLink
}
func (noopStreamService) EnsureTorrent(torrents TorrentService, spec *torrent.TorrentSpec, meta StreamMeta, allowCreate bool) (*torr.Torrent, error) {
	return nil, ErrStreamUnauthorized
}
func (noopStreamService) ParseFileIndex(index string, fileCount int) (int, error) {
	return 0, ErrStreamFileIndexInvalid
}
func (noopStreamService) NormalizePlaylistName(rawName, fallback string) string {
	return fallback + ".m3u"
}
func (noopPlaybackService) BuildAllPlaylist(host string, torrents TorrentService) PlaylistPayload {
	return PlaylistPayload{Name: "all.m3u", Body: "#EXTM3U\n"}
}
func (noopPlaybackService) BuildPlaylistByHash(hash, requestedName string, fromLast bool, host string, torrents TorrentService, viewed ViewedService) (PlaylistPayload, error) {
	return PlaylistPayload{}, ErrPlaylistTorrentNotFound
}
func (noopPlaybackService) BuildM3UFromStatus(tor *state.TorrentStatus, host string, fromLast bool, viewed ViewedService) string {
	return ""
}
func (noopPlaybackService) ResolvePlay(hash, index string, unauthorized bool, torrents TorrentService) (PlayTarget, error) {
	return PlayTarget{}, ErrPlayTorrentNotFound
}

func newNoopServices() *APIServices {
	return &APIServices{
		Torrents: noopTorrentService{},
		Settings: noopSettingsService{},
		Viewed:   noopViewedService{},
		System:   noopSystemService{},
		Search:   noopSearchService{},
		Media:    noopMediaService{},
		Modules:  noopModulesService{},
		Streams:  noopStreamService{},
		Playback: noopPlaybackService{},
	}
}

// SetServices injects API services from composition root.
func SetServices(s *APIServices) {
	if s == nil {
		return
	}

	s = withNoopFallbacks(s)

	apiServicesMu.Lock()
	apiServices = s
	apiServicesMu.Unlock()
}

func getServices() *APIServices {
	apiServicesMu.RLock()
	s := apiServices
	apiServicesMu.RUnlock()

	if s != nil {
		return withNoopFallbacks(s)
	}

	noop := newNoopServices()

	apiServicesMu.Lock()
	if apiServices == nil {
		apiServices = noop
	} else {
		noop = withNoopFallbacks(apiServices)
		apiServices = noop
	}
	apiServicesMu.Unlock()

	return noop
}

func withNoopFallbacks(s *APIServices) *APIServices {
	if s == nil {
		return newNoopServices()
	}

	if s.Torrents == nil {
		s.Torrents = noopTorrentService{}
	}

	if s.Settings == nil {
		s.Settings = noopSettingsService{}
	}

	if s.Viewed == nil {
		s.Viewed = noopViewedService{}
	}

	if s.System == nil {
		s.System = noopSystemService{}
	}

	if s.Search == nil {
		s.Search = noopSearchService{}
	}

	if s.Media == nil {
		s.Media = noopMediaService{}
	}

	if s.Modules == nil {
		s.Modules = noopModulesService{}
	}

	if s.Streams == nil {
		s.Streams = noopStreamService{}
	}

	if s.Playback == nil {
		s.Playback = noopPlaybackService{}
	}

	return s
}

func listTorrentStatuses(service TorrentService) []*state.TorrentStatus {
	list := service.List()
	if len(list) == 0 {
		return []*state.TorrentStatus{}
	}

	stats := make([]*state.TorrentStatus, 0, len(list))
	for _, tr := range list {
		stats = append(stats, tr.Status())
	}

	return stats
}
