package settings

import (
	"encoding/json"
)

type TorznabConfig struct {
	Host string
	Key  string
	Name string
}

type TMDBConfig struct {
	APIKey     string // TMDB API Key
	APIURL     string // Base API URL (default: https://api.themoviedb.org)
	ImageURL   string // Image URL (default: https://image.tmdb.org)
	ImageURLRu string // Image URL for Russian users (default: https://imagetmdb.com)
}

type BTSets struct {
	// Cache
	CacheSize       int64 // in byte, def 64 MB
	ReaderReadAHead int   // in percent, 5%-100%, [...S__X__E...] [S-E] not clean
	PreloadCache    int   // in percent

	// Disk
	UseDisk           bool
	TorrentsSavePath  string
	RemoveCacheOnDrop bool

	// Torrent
	ForceEncrypt             bool
	RetrackersMode           int  // 0 - don`t add, 1 - add retrackers (def), 2 - remove retrackers 3 - replace retrackers
	TorrentDisconnectTimeout int  // in seconds
	EnableDebug              bool // debug logs (includes library debug when enabled)
	ServiceOnlyDebug         bool // only V2 code debug logs, no library debug

	// DLNA
	EnableDLNA   bool
	FriendlyName string

	// Rutor
	EnableRutorSearch bool

	// Torznab
	EnableTorznabSearch bool
	TorznabUrls         []TorznabConfig

	// TMDB
	TMDBSettings TMDBConfig

	// BT Config
	EnableIPv6        bool
	DisableTCP        bool
	DisableUTP        bool
	DisableUPNP       bool
	DisableDHT        bool
	DisablePEX        bool
	DisableUpload     bool
	DownloadRateLimit int // in kb, 0 - inf
	UploadRateLimit   int // in kb, 0 - inf
	ConnectionsLimit  int
	PeersListenPort   int

	// HTTPS
	SslPort int
	SslCert string
	SslKey  string

	// Reader
	ResponsiveMode bool // enable Responsive reader (don't wait pieceComplete)
	// CoreProfile controls predefined kernel tuning presets.
	// Allowed: custom, low-end, balanced, high-throughput, nas.
	CoreProfile string
	// Stream admission control
	// MaxConcurrentStreams: 0 -> auto (2 * GOMAXPROCS), >0 -> fixed limit
	MaxConcurrentStreams int
	// StreamQueueSize: 0 -> auto (2 * effective max streams), >0 -> fixed queue limit
	StreamQueueSize int
	// StreamQueueWaitSec: max wait time for queue slot acquisition
	StreamQueueWaitSec int
	// Adaptive read-ahead bounds for streaming readers, in MB.
	// 0 values fallback to safe defaults.
	AdaptiveRAMinMB int
	AdaptiveRAMaxMB int
	// Warm disk cache (tier-2) controls, in MB/minutes.
	// 0 values fallback to safe defaults.
	WarmDiskCacheSizeMB int64
	WarmDiskCacheTTLMin int
	// Disk write pipeline controls.
	// DiskSyncPolicy: "none" | "periodic" | "always"
	DiskSyncPolicy string
	// DiskSyncIntervalMS is used only for "periodic" policy.
	DiskSyncIntervalMS int
	// DiskWriteBatchSize controls sequential write batching.
	DiskWriteBatchSize int
	// Background metadata finalization worker pool.
	// MetadataWorkers: 0 -> auto (max(2, GOMAXPROCS/2))
	MetadataWorkers int
	// MetadataQueueSize: 0 -> auto (256)
	MetadataQueueSize int
	// Preload worker pool.
	// PreloadWorkers: 0 -> auto (1)
	PreloadWorkers int
	// PreloadQueueSize: 0 -> auto (32)
	PreloadQueueSize int

	// FS
	ShowFSActiveTorr bool

	// Storage preferences
	StoreSettingsInJSON bool
	StoreViewedInJSON   bool

	// P2P Proxy
	EnableProxy bool
	ProxyHosts  []string
}

func (s *BTSets) String() string {
	buf, err := json.Marshal(s)
	if err != nil {
		return ""
	}

	return string(buf)
}
