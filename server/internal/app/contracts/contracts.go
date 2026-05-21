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

// TorrentSpec is an application-level torrent descriptor.
// It intentionally hides the torrent engine implementation type from transport handlers.
type TorrentSpec struct {
	hashHex string
	native  any
}

func NewTorrentSpec(hashHex string, native any) TorrentSpec {
	return TorrentSpec{
		hashHex: hashHex,
		native:  native,
	}
}

func (s TorrentSpec) HashHex() string {
	return s.hashHex
}

func (s TorrentSpec) Native() any {
	return s.native
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

// TorrentService defines application-level torrent use-cases consumed by HTTP handlers.
type TorrentService interface {
	Add(spec TorrentSpec, title, poster, data, category string) (TorrentHandle, error)
	Get(hash string) TorrentHandle
	Status(tor TorrentHandle) *state.TorrentStatus
	StatusByHash(hash string) (*state.TorrentStatus, bool)
	Set(hash, title, poster, category, data string) TorrentHandle
	SaveToDB(tor TorrentHandle)
	Remove(hash string)
	List() []TorrentHandle
	Statuses() []*state.TorrentStatus
	ListHashes() []string
	Drop(hash string)
	IsStored(tor TorrentHandle) bool
	DropReadiness(hash string) DropReadiness
	CacheStateByHash(hash string) (any, bool)
	EnqueuePreload(tor TorrentHandle, index int) bool
	EnqueueMetadataFinalize(tor TorrentHandle, spec *TorrentSpec, saveToDB bool) bool
	LoadFromDB(tor TorrentHandle) TorrentHandle
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

// StreamService defines stream orchestration helpers used by transport handlers.
type StreamService interface {
	ParseLink(link, title, poster, category string) (TorrentSpec, StreamMeta, error)
	ParseTorrentFile(reader io.Reader) (TorrentSpec, error)
	EnsureTorrent(torrents TorrentService, spec TorrentSpec, meta StreamMeta, allowCreate bool) (TorrentHandle, error)
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
	Torrent   TorrentHandle
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
