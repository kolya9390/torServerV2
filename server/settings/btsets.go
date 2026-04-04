package settings

import (
	"encoding/json"
	"io"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"server/log"
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
	EnableDebug              bool // debug logs

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
	StoreSettingsInJson bool
	StoreViewedInJson   bool

	// P2P Proxy
	EnableProxy bool
	ProxyHosts  []string
}

func (v *BTSets) String() string {
	buf, _ := json.Marshal(v)
	return string(buf)
}

var BTsets *BTSets
var btsetsMu sync.RWMutex

func SetBTSets(sets *BTSets) {
	if ReadOnly {
		return
	}
	input := *sets
	sets.CoreProfile = normalizeCoreProfile(sets.CoreProfile)
	if sets.CoreProfile != "custom" {
		applyCoreProfilePreset(sets, sets.CoreProfile)
		applyCoreProfileOverrides(sets, &input)
	}
	// failsafe checks (use defaults)
	if sets.CacheSize == 0 {
		sets.CacheSize = 64 * 1024 * 1024
	}
	if sets.ConnectionsLimit == 0 {
		sets.ConnectionsLimit = 25
	}
	if sets.TorrentDisconnectTimeout == 0 {
		sets.TorrentDisconnectTimeout = 30
	}
	if sets.StreamQueueWaitSec <= 0 {
		sets.StreamQueueWaitSec = 3
	}
	if sets.MaxConcurrentStreams < 0 {
		sets.MaxConcurrentStreams = 0
	}
	if sets.StreamQueueSize < 0 {
		sets.StreamQueueSize = 0
	}
	if sets.AdaptiveRAMinMB < 0 {
		sets.AdaptiveRAMinMB = 0
	}
	if sets.AdaptiveRAMaxMB < 0 {
		sets.AdaptiveRAMaxMB = 0
	}
	if sets.AdaptiveRAMaxMB > 0 && sets.AdaptiveRAMinMB > sets.AdaptiveRAMaxMB {
		sets.AdaptiveRAMinMB = sets.AdaptiveRAMaxMB
	}
	if sets.WarmDiskCacheSizeMB < 0 {
		sets.WarmDiskCacheSizeMB = 0
	}
	if sets.WarmDiskCacheTTLMin < 0 {
		sets.WarmDiskCacheTTLMin = 0
	}
	if sets.DiskSyncPolicy == "" {
		sets.DiskSyncPolicy = "periodic"
	}
	switch strings.ToLower(sets.DiskSyncPolicy) {
	case "none", "periodic", "always":
		sets.DiskSyncPolicy = strings.ToLower(sets.DiskSyncPolicy)
	default:
		sets.DiskSyncPolicy = "periodic"
	}
	if sets.DiskSyncIntervalMS <= 0 {
		sets.DiskSyncIntervalMS = 1000
	}
	if sets.DiskWriteBatchSize <= 0 {
		sets.DiskWriteBatchSize = 16
	}
	if sets.MetadataWorkers < 0 {
		sets.MetadataWorkers = 0
	}
	if sets.MetadataQueueSize < 0 {
		sets.MetadataQueueSize = 0
	}
	if sets.PreloadWorkers < 0 {
		sets.PreloadWorkers = 0
	}
	if sets.PreloadQueueSize < 0 {
		sets.PreloadQueueSize = 0
	}

	if sets.ReaderReadAHead < 5 {
		sets.ReaderReadAHead = 5
	}
	if sets.ReaderReadAHead > 100 {
		sets.ReaderReadAHead = 100
	}

	if sets.PreloadCache < 0 {
		sets.PreloadCache = 0
	}
	if sets.PreloadCache > 100 {
		sets.PreloadCache = 100
	}

	if sets.TorrentsSavePath == "" {
		sets.UseDisk = false
	} else if sets.UseDisk {
		_ = filepath.WalkDir(sets.TorrentsSavePath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && strings.ToLower(d.Name()) == ".tsc" {
				sets.TorrentsSavePath = path
				log.TLogln("Find directory \"" + sets.TorrentsSavePath + "\", use as cache dir")
				return io.EOF
			}
			if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		})
	}

	btsetsMu.Lock()
	BTsets = sets
	btsetsMu.Unlock()
	buf, err := json.Marshal(sets)
	if err != nil {
		log.TLogln("Error marshal btsets", err)
		return
	}
	tdb.Set("Settings", "BitTorr", buf)
}

func SetDefaultConfig() {
	sets := new(BTSets)
	sets.CacheSize = 64 * 1024 * 1024 // 64 MB
	sets.PreloadCache = 50
	sets.ConnectionsLimit = 25
	sets.RetrackersMode = 1
	sets.TorrentDisconnectTimeout = 30
	sets.ReaderReadAHead = 95 // 95%
	sets.ResponsiveMode = true
	sets.CoreProfile = "custom"
	sets.MaxConcurrentStreams = 0 // auto
	sets.StreamQueueSize = 0      // auto
	sets.StreamQueueWaitSec = 3
	sets.AdaptiveRAMinMB = 4
	sets.AdaptiveRAMaxMB = 64
	sets.WarmDiskCacheSizeMB = 0 // auto
	sets.WarmDiskCacheTTLMin = 180
	sets.DiskSyncPolicy = "periodic"
	sets.DiskSyncIntervalMS = 1000
	sets.DiskWriteBatchSize = 16
	sets.MetadataWorkers = 0   // auto
	sets.MetadataQueueSize = 0 // auto
	sets.PreloadWorkers = 0    // auto
	sets.PreloadQueueSize = 0  // auto
	sets.ShowFSActiveTorr = true
	sets.StoreSettingsInJson = true
	// Set default TMDB settings
	sets.TMDBSettings = TMDBConfig{
		APIKey:     "",
		APIURL:     "https://api.themoviedb.org",
		ImageURL:   "https://image.tmdb.org",
		ImageURLRu: "https://imagetmdb.com",
	}
	btsetsMu.Lock()
	BTsets = sets
	btsetsMu.Unlock()
	if !ReadOnly {
		buf, err := json.Marshal(sets)
		if err != nil {
			log.TLogln("Error marshal btsets", err)
			return
		}
		tdb.Set("Settings", "BitTorr", buf)
	}
	//Proxy
	sets.EnableProxy = false
	sets.ProxyHosts = []string{"*themoviedb.org", "*tmdb.org", "rutor.info"}
}

func loadBTSets() {
	buf := tdb.Get("Settings", "BitTorr")
	if len(buf) > 0 {
		loaded := new(BTSets)
		err := json.Unmarshal(buf, loaded)
		if err == nil {
			if loaded.ReaderReadAHead < 5 {
				loaded.ReaderReadAHead = 5
			}
			if loaded.CacheSize == 0 {
				loaded.CacheSize = 64 * 1024 * 1024
			}
			if loaded.ConnectionsLimit == 0 {
				loaded.ConnectionsLimit = 25
			}
			if loaded.TorrentDisconnectTimeout == 0 {
				loaded.TorrentDisconnectTimeout = 30
			}
			loaded.CoreProfile = normalizeCoreProfile(loaded.CoreProfile)
			if loaded.StreamQueueWaitSec <= 0 {
				loaded.StreamQueueWaitSec = 3
			}
			if loaded.MaxConcurrentStreams < 0 {
				loaded.MaxConcurrentStreams = 0
			}
			if loaded.StreamQueueSize < 0 {
				loaded.StreamQueueSize = 0
			}
			if loaded.AdaptiveRAMinMB < 0 {
				loaded.AdaptiveRAMinMB = 0
			}
			if loaded.AdaptiveRAMaxMB < 0 {
				loaded.AdaptiveRAMaxMB = 0
			}
			if loaded.AdaptiveRAMinMB == 0 {
				loaded.AdaptiveRAMinMB = 4
			}
			if loaded.AdaptiveRAMaxMB == 0 {
				loaded.AdaptiveRAMaxMB = 64
			}
			if loaded.AdaptiveRAMinMB > loaded.AdaptiveRAMaxMB {
				loaded.AdaptiveRAMinMB = loaded.AdaptiveRAMaxMB
			}
			if loaded.WarmDiskCacheSizeMB < 0 {
				loaded.WarmDiskCacheSizeMB = 0
			}
			if loaded.WarmDiskCacheTTLMin <= 0 {
				loaded.WarmDiskCacheTTLMin = 180
			}
			if loaded.DiskSyncPolicy == "" {
				loaded.DiskSyncPolicy = "periodic"
			}
			switch strings.ToLower(loaded.DiskSyncPolicy) {
			case "none", "periodic", "always":
				loaded.DiskSyncPolicy = strings.ToLower(loaded.DiskSyncPolicy)
			default:
				loaded.DiskSyncPolicy = "periodic"
			}
			if loaded.DiskSyncIntervalMS <= 0 {
				loaded.DiskSyncIntervalMS = 1000
			}
			if loaded.DiskWriteBatchSize <= 0 {
				loaded.DiskWriteBatchSize = 16
			}
			if loaded.MetadataWorkers < 0 {
				loaded.MetadataWorkers = 0
			}
			if loaded.MetadataQueueSize < 0 {
				loaded.MetadataQueueSize = 0
			}
			if loaded.PreloadWorkers < 0 {
				loaded.PreloadWorkers = 0
			}
			if loaded.PreloadQueueSize < 0 {
				loaded.PreloadQueueSize = 0
			}
			// Set default TMDB settings if missing (for existing configs)
			if loaded.TMDBSettings.APIURL == "" {
				loaded.TMDBSettings = TMDBConfig{
					APIKey:     "",
					APIURL:     "https://api.themoviedb.org",
					ImageURL:   "https://image.tmdb.org",
					ImageURLRu: "https://imagetmdb.com",
				}
			}
			btsetsMu.Lock()
			BTsets = loaded
			btsetsMu.Unlock()
			return
		}
		log.TLogln("Error unmarshal btsets", err)
	}
	// initialize defaults on error
	SetDefaultConfig()
}

func normalizeCoreProfile(profile string) string {
	p := strings.ToLower(strings.TrimSpace(profile))
	switch p {
	case "", "custom":
		return "custom"
	case "low-end", "balanced", "high-throughput", "nas":
		return p
	default:
		return "custom"
	}
}

func applyCoreProfilePreset(sets *BTSets, profile string) {
	cpus := runtime.GOMAXPROCS(0)
	if cpus < 1 {
		cpus = 1
	}

	switch profile {
	case "low-end":
		sets.CacheSize = 32 * 1024 * 1024
		sets.PreloadCache = 25
		sets.ConnectionsLimit = 12
		sets.MaxConcurrentStreams = 1
		sets.StreamQueueSize = 2
		sets.StreamQueueWaitSec = 2
		sets.AdaptiveRAMinMB = 2
		sets.AdaptiveRAMaxMB = 16
		sets.WarmDiskCacheSizeMB = 256
		sets.WarmDiskCacheTTLMin = 60
		sets.MetadataWorkers = 1
		sets.MetadataQueueSize = 64
		sets.PreloadWorkers = 1
		sets.PreloadQueueSize = 8
		sets.DiskSyncPolicy = "periodic"
		sets.DiskSyncIntervalMS = 800
		sets.DiskWriteBatchSize = 8
	case "high-throughput":
		sets.CacheSize = 256 * 1024 * 1024
		sets.PreloadCache = 70
		sets.ConnectionsLimit = maxInt(cpus*20, 80)
		sets.MaxConcurrentStreams = maxInt(cpus*2, 4)
		sets.StreamQueueSize = maxInt(cpus*4, 12)
		sets.StreamQueueWaitSec = 4
		sets.AdaptiveRAMinMB = 8
		sets.AdaptiveRAMaxMB = 128
		sets.WarmDiskCacheSizeMB = 4096
		sets.WarmDiskCacheTTLMin = 360
		sets.MetadataWorkers = maxInt(cpus, 4)
		sets.MetadataQueueSize = 512
		sets.PreloadWorkers = maxInt(cpus/2, 2)
		sets.PreloadQueueSize = 64
		sets.DiskSyncPolicy = "periodic"
		sets.DiskSyncIntervalMS = 1500
		sets.DiskWriteBatchSize = 32
	case "nas":
		sets.CacheSize = 128 * 1024 * 1024
		sets.PreloadCache = 55
		sets.ConnectionsLimit = maxInt(cpus*12, 40)
		sets.MaxConcurrentStreams = maxInt(cpus, 2)
		sets.StreamQueueSize = maxInt(cpus*3, 8)
		sets.StreamQueueWaitSec = 4
		sets.AdaptiveRAMinMB = 4
		sets.AdaptiveRAMaxMB = 64
		sets.WarmDiskCacheSizeMB = 8192
		sets.WarmDiskCacheTTLMin = 720
		sets.MetadataWorkers = maxInt(cpus/2, 2)
		sets.MetadataQueueSize = 256
		sets.PreloadWorkers = 1
		sets.PreloadQueueSize = 32
		sets.DiskSyncPolicy = "periodic"
		sets.DiskSyncIntervalMS = 2500
		sets.DiskWriteBatchSize = 24
	default: // balanced
		sets.CacheSize = 64 * 1024 * 1024
		sets.PreloadCache = 50
		sets.ConnectionsLimit = 25
		sets.MaxConcurrentStreams = 0
		sets.StreamQueueSize = 0
		sets.StreamQueueWaitSec = 3
		sets.AdaptiveRAMinMB = 4
		sets.AdaptiveRAMaxMB = 64
		sets.WarmDiskCacheSizeMB = 0
		sets.WarmDiskCacheTTLMin = 180
		sets.MetadataWorkers = 0
		sets.MetadataQueueSize = 0
		sets.PreloadWorkers = 0
		sets.PreloadQueueSize = 0
		sets.DiskSyncPolicy = "periodic"
		sets.DiskSyncIntervalMS = 1000
		sets.DiskWriteBatchSize = 16
	}
}

func applyCoreProfileOverrides(dst, src *BTSets) {
	if src == nil || dst == nil {
		return
	}
	if src.CacheSize > 0 {
		dst.CacheSize = src.CacheSize
	}
	if src.ReaderReadAHead > 0 {
		dst.ReaderReadAHead = src.ReaderReadAHead
	}
	if src.PreloadCache > 0 {
		dst.PreloadCache = src.PreloadCache
	}
	if src.ConnectionsLimit > 0 {
		dst.ConnectionsLimit = src.ConnectionsLimit
	}
	if src.TorrentDisconnectTimeout > 0 {
		dst.TorrentDisconnectTimeout = src.TorrentDisconnectTimeout
	}
	if src.MaxConcurrentStreams > 0 {
		dst.MaxConcurrentStreams = src.MaxConcurrentStreams
	}
	if src.StreamQueueSize > 0 {
		dst.StreamQueueSize = src.StreamQueueSize
	}
	if src.StreamQueueWaitSec > 0 {
		dst.StreamQueueWaitSec = src.StreamQueueWaitSec
	}
	if src.AdaptiveRAMinMB > 0 {
		dst.AdaptiveRAMinMB = src.AdaptiveRAMinMB
	}
	if src.AdaptiveRAMaxMB > 0 {
		dst.AdaptiveRAMaxMB = src.AdaptiveRAMaxMB
	}
	if src.WarmDiskCacheSizeMB > 0 {
		dst.WarmDiskCacheSizeMB = src.WarmDiskCacheSizeMB
	}
	if src.WarmDiskCacheTTLMin > 0 {
		dst.WarmDiskCacheTTLMin = src.WarmDiskCacheTTLMin
	}
	if src.DiskSyncPolicy != "" {
		dst.DiskSyncPolicy = src.DiskSyncPolicy
	}
	if src.DiskSyncIntervalMS > 0 {
		dst.DiskSyncIntervalMS = src.DiskSyncIntervalMS
	}
	if src.DiskWriteBatchSize > 0 {
		dst.DiskWriteBatchSize = src.DiskWriteBatchSize
	}
	if src.MetadataWorkers > 0 {
		dst.MetadataWorkers = src.MetadataWorkers
	}
	if src.MetadataQueueSize > 0 {
		dst.MetadataQueueSize = src.MetadataQueueSize
	}
	if src.PreloadWorkers > 0 {
		dst.PreloadWorkers = src.PreloadWorkers
	}
	if src.PreloadQueueSize > 0 {
		dst.PreloadQueueSize = src.PreloadQueueSize
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
