package contracts

import (
	"errors"
	"io"
	"net/http"
	"time"
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
	Status() *TorrentStatus
	State() TorrentState
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
	Status(tor TorrentHandle) *TorrentStatus
	StatusByHash(hash string) (*TorrentStatus, bool)
	IsStored(tor TorrentHandle) bool
}

// TorrentCatalog reads torrent collection views.
type TorrentCatalog interface {
	List() []TorrentHandle
	Statuses() []*TorrentStatus
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

// PlaybackAdmissionDecision reports whether a playback request should start torrent work.
type PlaybackAdmissionDecision struct {
	Allowed       bool
	RetryAfterSec int
	Reason        string
}

// SettingsService defines settings use-cases for API handlers.
type SettingsService interface {
	Current() *Settings
	Set(*Settings)
	SetDefault()
	ReadOnly() bool
	GetStoragePreferences() map[string]any
	SetStoragePreferences(map[string]any) error
	TMDBConfig() (TMDBConfig, bool)
	BuildPlayURL(hash, fileID string) string
	EnableDLNA() bool
	EnableDebug() bool
}

// ViewedService defines viewed-history operations consumed by handlers.
type ViewedService interface {
	SetViewed(v *ViewedItem)
	RemoveViewed(v *ViewedItem)
	ListViewed(hash string) []*ViewedItem
}

// SystemService defines process-level operations used by API handlers.
type SystemService interface {
	Shutdown()
}

// SearchService defines external search integrations.
type SearchService interface {
	EnableTorznabSearch() bool
	TorznabSearch(query string, index int) []*SearchResult
	TorznabTest(host, key string) error
}

// MediaService defines media metadata operations used by API handlers.
type MediaService interface {
	ProbePlayURL(hash, fileID string) (MediaProbe, error)
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

// SearchResult is an application-owned torrent search result DTO.
// Keep JSON tags compatible with the legacy Torznab HTTP response shape.
type SearchResult struct {
	Title      string    `json:"title,omitempty"`
	Name       string    `json:"name,omitempty"`
	Link       string    `json:"link,omitempty"`
	Magnet     string    `json:"magnet,omitempty"`
	Hash       string    `json:"hash,omitempty"`
	Size       string    `json:"size,omitempty"`
	Seed       int       `json:"seed,omitempty"`
	Peer       int       `json:"peer,omitempty"`
	CreateDate time.Time `json:"createDate"`
	Categories []string  `json:"categories,omitempty"`
	Year       int       `json:"year,omitempty"`
}

// TorrentState is an application-owned torrent lifecycle state.
// Numeric values intentionally match the legacy torr/state JSON values.
type TorrentState int

const (
	TorrentAdded TorrentState = iota
	TorrentGettingInfo
	TorrentPreload
	TorrentWorking
	TorrentClosed
	TorrentInDB
)

// TorrentStatus is an application-owned torrent runtime read-model.
// JSON tags preserve the legacy torr/state.TorrentStatus response shape.
type TorrentStatus struct {
	Title               string         `json:"title"`
	Category            string         `json:"category"`
	Poster              string         `json:"poster"`
	Data                string         `json:"data,omitempty"`
	Timestamp           int64          `json:"timestamp"`
	Name                string         `json:"name,omitempty"`
	Hash                string         `json:"hash,omitempty"`
	TorrsHash           string         `json:"torrs_hash,omitempty"`
	Stat                TorrentState   `json:"stat"`
	StatString          string         `json:"stat_string"`
	LoadedSize          int64          `json:"loaded_size,omitempty"`
	TorrentSize         int64          `json:"torrent_size,omitempty"`
	PreloadedBytes      int64          `json:"preloaded_bytes,omitempty"`
	PreloadSize         int64          `json:"preload_size,omitempty"`
	DownloadSpeed       float64        `json:"download_speed,omitempty"`
	UploadSpeed         float64        `json:"upload_speed,omitempty"`
	TotalPeers          int            `json:"total_peers,omitempty"`
	PendingPeers        int            `json:"pending_peers,omitempty"`
	ActivePeers         int            `json:"active_peers,omitempty"`
	ConnectedSeeders    int            `json:"connected_seeders,omitempty"`
	HalfOpenPeers       int            `json:"half_open_peers,omitempty"`
	BytesWritten        int64          `json:"bytes_written,omitempty"`
	BytesWrittenData    int64          `json:"bytes_written_data,omitempty"`
	BytesRead           int64          `json:"bytes_read,omitempty"`
	BytesReadData       int64          `json:"bytes_read_data,omitempty"`
	BytesReadUsefulData int64          `json:"bytes_read_useful_data,omitempty"`
	ChunksWritten       int64          `json:"chunks_written,omitempty"`
	ChunksRead          int64          `json:"chunks_read,omitempty"`
	ChunksReadUseful    int64          `json:"chunks_read_useful,omitempty"`
	ChunksReadWasted    int64          `json:"chunks_read_wasted,omitempty"`
	PiecesDirtiedGood   int64          `json:"pieces_dirtied_good,omitempty"`
	PiecesDirtiedBad    int64          `json:"pieces_dirtied_bad,omitempty"`
	DurationSeconds     float64        `json:"duration_seconds,omitempty"`
	BitRate             string         `json:"bit_rate,omitempty"`
	FileStats           []*TorrentFile `json:"file_stats,omitempty"`
}

// TorrentFile is an application-owned torrent file status read-model.
type TorrentFile struct {
	ID     int    `json:"id,omitempty"` //nolint:staticcheck // json tag preserves API compatibility
	Path   string `json:"path,omitempty"`
	Length int64  `json:"length,omitempty"`
}

// MediaProbe is an application-owned media metadata DTO.
// It preserves the legacy ffprobe JSON response shape without exposing the ffprobe package.
type MediaProbe map[string]any

// ViewedItem is an application-owned viewed-history DTO.
// Keep JSON tags compatible with the legacy settings.Viewed HTTP contract.
type ViewedItem struct {
	Hash      string `json:"hash"`
	FileIndex int    `json:"file_index"`
}

// TorznabConfig is an application-owned Torznab endpoint setting.
type TorznabConfig struct {
	Host string
	Key  string
	Name string
}

// TMDBConfig is an application-owned TMDB API setting.
type TMDBConfig struct {
	APIKey     string // #nosec G117 -- DTO field for operator-provided config, not a hardcoded credential.
	APIURL     string
	ImageURL   string
	ImageURLRu string
}

// Settings is an application-owned server settings DTO.
// Exported field names intentionally preserve the legacy BTSets JSON shape.
type Settings struct {
	CacheSize       int64
	ReaderReadAHead int
	PreloadCache    int

	UseDisk           bool
	TorrentsSavePath  string
	RemoveCacheOnDrop bool

	ForceEncrypt                    bool
	RetrackersMode                  int
	TorrentDisconnectTimeout        int
	EnableDebug                     bool
	ServiceOnlyDebug                bool
	DebugEstablishedConnsOverride   int
	DebugTotalHalfOpenConnsOverride int
	DebugTrackerBudgetOverride      int
	DebugStablePeerCap              int

	EnableDLNA   bool
	FriendlyName string

	EnableRutorSearch bool

	EnableTorznabSearch bool
	TorznabUrls         []TorznabConfig

	TMDBSettings TMDBConfig

	EnableIPv6        bool
	DisableTCP        bool
	DisableUTP        bool
	DisableUPNP       bool
	DisableDHT        bool
	DisablePEX        bool
	DisableUpload     bool
	DownloadRateLimit int
	UploadRateLimit   int
	ConnectionsLimit  int
	PeersListenPort   int
	EnableLPD         bool
	LPDIPv6           bool

	SslPort int
	SslCert string
	SslKey  string

	ResponsiveMode            bool
	CoreProfile               string
	MaxConcurrentStreams      int
	StreamQueueSize           int
	StreamQueueWaitSec        int
	MaxUniquePlaybackTorrents int
	AdaptiveRAMinMB           int
	AdaptiveRAMaxMB           int
	WarmDiskCacheSizeMB       int64
	WarmDiskCacheTTLMin       int
	DiskSyncPolicy            string
	DiskSyncIntervalMS        int
	DiskWriteBatchSize        int
	MetadataWorkers           int
	MetadataQueueSize         int
	PreloadWorkers            int
	PreloadQueueSize          int

	ShowFSActiveTorr bool

	StoreSettingsInJSON bool
	StoreViewedInJSON   bool

	EnableProxy bool
	ProxyHosts  []string
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
	BuildM3UFromStatus(tor *TorrentStatus, host string, fromLast bool, viewed ViewedService) string
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
