package contracts

import (
	"errors"
	"io"
	"net/http"
	"time"

	goffprobe "gopkg.in/vansante/go-ffprobe.v2"

	sets "server/settings"
	"server/torr/state"
	"server/torznab"
)

// TorrentSpecPayload is an adapter-owned payload for a torrent engine implementation.
// Application contracts keep it typed so callers cannot smuggle arbitrary unstructured values.
type TorrentSpecPayload interface {
	TorrentSpecPayload()
}

// TorrentSpec is an application-level torrent descriptor.
// It intentionally hides the torrent engine implementation type from transport handlers.
type TorrentSpec struct {
	hashHex string
	payload TorrentSpecPayload
}

func NewTorrentSpec(hashHex string, payload TorrentSpecPayload) TorrentSpec {
	return TorrentSpec{
		hashHex: hashHex,
		payload: payload,
	}
}

func (s TorrentSpec) HashHex() string {
	return s.hashHex
}

func (s TorrentSpec) Payload() TorrentSpecPayload {
	return s.payload
}

// TorrentHandle is the transport-facing view of an active torrent.
type TorrentHandle interface {
	Status() *state.TorrentStatus
	State() state.TorrentStat
	HashHex() string
	Name() string
	FileCount() int
	Ready() bool
	EnsureTitleFromInfo()
	Metadata() StreamMeta
	Stream(index int, request *http.Request, writer http.ResponseWriter) error
}

// TorrentCreator creates or activates torrent handles from application specs.
type TorrentCreator interface {
	Add(spec TorrentSpec, title, poster, data, category string) (TorrentHandle, error)
}

// TorrentLookup reads active torrent handles and derived runtime state.
type TorrentLookup interface {
	Get(hash string) TorrentHandle
	Status(tor TorrentHandle) *state.TorrentStatus
	StatusByHash(hash string) (*state.TorrentStatus, bool)
	IsStored(tor TorrentHandle) bool
}

// TorrentCatalog reads torrent collection views.
type TorrentCatalog interface {
	List() []TorrentHandle
	Statuses() []*state.TorrentStatus
	ListHashes() []string
}

// TorrentMutation applies user-visible torrent changes.
type TorrentMutation interface {
	Set(hash, title, poster, category, data string) TorrentHandle
	SaveToDB(tor TorrentHandle)
	Remove(hash string)
	Drop(hash string)
	DropReadiness(hash string) DropReadiness
	EnqueuePreload(tor TorrentHandle, index int) bool
	EnqueueMetadataFinalize(tor TorrentHandle, spec *TorrentSpec, saveToDB bool) bool
}

// TorrentStorage reads storage/cache state for torrent-backed endpoints.
type TorrentStorage interface {
	CacheStateByHash(hash string) (any, bool)
}

// TorrentLoader activates a stored torrent into a playable runtime handle.
type TorrentLoader interface {
	LoadFromDB(tor TorrentHandle) TorrentHandle
}

// TorrentQueryService is the read-side torrent capability needed by HTTP handlers.
type TorrentQueryService interface {
	TorrentLookup
	TorrentCatalog
	TorrentStorage
}

// TorrentCommandService is the write-side torrent capability needed by HTTP handlers.
type TorrentCommandService interface {
	TorrentCreator
	TorrentMutation
}

// TorrentStreamService contains the torrent capabilities needed by stream orchestration.
type TorrentStreamService interface {
	TorrentCreator
	TorrentLookup
	TorrentLoader
}

// TorrentStreamActions contains side-effect operations used by stream endpoints.
type TorrentStreamActions interface {
	SaveToDB(tor TorrentHandle)
	EnqueuePreload(tor TorrentHandle, index int) bool
}

// TorrentPlaylistService contains the torrent capabilities needed to build playlists.
type TorrentPlaylistService interface {
	TorrentCatalog
	TorrentLookup
	TorrentLoader
}

// TorrentPlayService contains the torrent capabilities needed to resolve direct playback.
type TorrentPlayService interface {
	TorrentCreator
	TorrentLookup
}

// TorrentService defines the full torrent use-case surface at application composition boundaries.
type TorrentService interface {
	TorrentQueryService
	TorrentCommandService
	TorrentLoader
}

// DropReadiness reports whether a torrent can be safely dropped from the API layer.
type DropReadiness struct {
	ActiveReaders       int
	ActiveStreams       int32
	RecentStreamElapsed time.Duration
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
	// ErrTorrentSpecInvalid indicates that an application torrent spec cannot be used by the backend.
	ErrTorrentSpecInvalid = errors.New("torrent spec is invalid")

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

// TorrentParserService parses torrent inputs into application-owned descriptors.
type TorrentParserService interface {
	ParseLink(link, title, poster, category string) (TorrentSpec, StreamMeta, error)
	ParseTorrentFile(reader io.Reader) (TorrentSpec, error)
}

// StreamOrchestratorService prepares active torrent handles for stream operations.
type StreamOrchestratorService interface {
	EnsureTorrent(torrents TorrentStreamService, spec TorrentSpec, meta StreamMeta, allowCreate bool) (TorrentHandle, error)
}

// StreamHelperService contains pure stream request helpers.
type StreamHelperService interface {
	ParseFileIndex(index string, fileCount int) (int, error)
	NormalizePlaylistName(rawName, fallback string) string
}

// StreamService is the default composition implementation for stream-related capabilities.
type StreamService interface {
	TorrentParserService
	StreamOrchestratorService
	StreamHelperService
}

// PlaylistPayload contains generated M3U payload details.
type PlaylistPayload struct {
	Name string
	Hash string
	Body string
}

// PlayTarget contains resolved torrent/file index for play endpoint.
type PlayTarget struct {
	Torrent   TorrentHandle
	FileIndex int
}

// PlaybackService contains playlist and play orchestration logic.
type PlaybackService interface {
	BuildAllPlaylist(host string, torrents TorrentPlaylistService) PlaylistPayload
	BuildPlaylistByHash(hash, requestedName string, fromLast bool, host string, torrents TorrentPlaylistService, viewed ViewedService) (PlaylistPayload, error)
	BuildM3UFromStatus(tor *state.TorrentStatus, host string, fromLast bool, viewed ViewedService) string
	ResolvePlay(hash, index string, unauthorized bool, torrents TorrentPlayService) (PlayTarget, error)
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
