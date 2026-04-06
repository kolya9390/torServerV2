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

// maxInt returns the maximum of two integers.
func maxInt(a, b int) int {
	if a > b {
		return a
	}

	return b
}

// lowEndProfile defines a preset optimized for resource-constrained devices.
var lowEndProfile = BTSets{
	CacheSize:          32 * 1024 * 1024,
	PreloadCache:       25,
	ConnectionsLimit:   12,
	MaxConcurrentStreams: 1,
	StreamQueueSize:    2,
	StreamQueueWaitSec: 2,
	AdaptiveRAMinMB:    2,
	AdaptiveRAMaxMB:    16,
	WarmDiskCacheSizeMB: 256,
	WarmDiskCacheTTLMin: 60,
	MetadataWorkers:    1,
	MetadataQueueSize:  64,
	PreloadWorkers:     1,
	PreloadQueueSize:   8,
	DiskSyncPolicy:     "periodic",
	DiskSyncIntervalMS: 800,
	DiskWriteBatchSize: 8,
}

// balancedProfile defines the default balanced preset.
var balancedProfile = BTSets{
	CacheSize:          64 * 1024 * 1024,
	PreloadCache:       50,
	ConnectionsLimit:   25,
	MaxConcurrentStreams: 0,
	StreamQueueSize:    0,
	StreamQueueWaitSec: 3,
	AdaptiveRAMinMB:    4,
	AdaptiveRAMaxMB:    64,
	WarmDiskCacheSizeMB: 0,
	WarmDiskCacheTTLMin: 180,
	MetadataWorkers:    0,
	MetadataQueueSize:  0,
	PreloadWorkers:     0,
	PreloadQueueSize:   0,
	DiskSyncPolicy:     "periodic",
	DiskSyncIntervalMS: 1000,
	DiskWriteBatchSize: 16,
}

// highThroughputProfile defines a preset optimized for high-performance systems.
var highThroughputProfile = BTSets{
	CacheSize:          256 * 1024 * 1024,
	PreloadCache:       70,
	ConnectionsLimit:   0, // computed at apply time
	MaxConcurrentStreams: 0, // computed at apply time
	StreamQueueSize:    0, // computed at apply time
	StreamQueueWaitSec: 4,
	AdaptiveRAMinMB:    8,
	AdaptiveRAMaxMB:    128,
	WarmDiskCacheSizeMB: 4096,
	WarmDiskCacheTTLMin: 360,
	MetadataWorkers:    0, // computed at apply time
	MetadataQueueSize:  512,
	PreloadWorkers:     0, // computed at apply time
	PreloadQueueSize:   64,
	DiskSyncPolicy:     "periodic",
	DiskSyncIntervalMS: 1500,
	DiskWriteBatchSize: 32,
}

// nasProfile defines a preset optimized for NAS devices.
var nasProfile = BTSets{
	CacheSize:          128 * 1024 * 1024,
	PreloadCache:       55,
	ConnectionsLimit:   0, // computed at apply time
	MaxConcurrentStreams: 0, // computed at apply time
	StreamQueueSize:    0, // computed at apply time
	StreamQueueWaitSec: 4,
	AdaptiveRAMinMB:    4,
	AdaptiveRAMaxMB:    64,
	WarmDiskCacheSizeMB: 8192,
	WarmDiskCacheTTLMin: 720,
	MetadataWorkers:    0, // computed at apply time
	MetadataQueueSize:  256,
	PreloadWorkers:     1,
	PreloadQueueSize:   32,
	DiskSyncPolicy:     "periodic",
	DiskSyncIntervalMS: 2500,
	DiskWriteBatchSize: 24,
}

// profilePresets maps profile names to their preset configurations.
var profilePresets = map[string]BTSets{
	"low-end":          lowEndProfile,
	"balanced":         balancedProfile,
	"high-throughput":  highThroughputProfile,
	"nas":              nasProfile,
}

// computeCPUFields applies runtime-computed CPU-dependent values to preset.
// It returns a modified copy with fields calculated based on GOMAXPROCS.
func computeCPUFields(preset BTSets) BTSets {
	cpus := max(runtime.GOMAXPROCS(0), 1)

	// High-throughput profile: CacheSize == 256MB signals CPU-dependent fields
	if preset.ConnectionsLimit == 0 && preset.CacheSize == 256*1024*1024 {
		preset.ConnectionsLimit = maxInt(cpus*20, 80)
		preset.MaxConcurrentStreams = maxInt(cpus*2, 4)
		preset.StreamQueueSize = maxInt(cpus*4, 12)
		preset.MetadataWorkers = maxInt(cpus, 4)
		preset.PreloadWorkers = maxInt(cpus/2, 2)
		return preset
	}

	// NAS profile: CacheSize == 128MB signals CPU-dependent fields
	if preset.ConnectionsLimit == 0 && preset.CacheSize == 128*1024*1024 {
		preset.ConnectionsLimit = maxInt(cpus*12, 40)
		preset.MaxConcurrentStreams = maxInt(cpus, 2)
		preset.StreamQueueSize = maxInt(cpus*3, 8)
		preset.MetadataWorkers = maxInt(cpus/2, 2)
	}

	return preset
}

// fieldCopyRule defines when a field should be copied from preset to dst.
type fieldCopyRule struct {
	shouldCopy func(preset BTSets, cacheSize int64) bool
	apply      func(dst *BTSets, preset BTSets)
}

// balancedZero indicates a field with value 0 from the balanced profile should be applied.
func balancedZero(preset BTSets, cacheSize int64) bool {
	return preset.CacheSize == cacheSize
}

// copyRules defines the table-driven field copying logic for applyProfileFields.
var copyRules = []fieldCopyRule{
	{
		shouldCopy: func(p BTSets, _ int64) bool { return p.CacheSize > 0 },
		apply:      func(dst *BTSets, p BTSets) { dst.CacheSize = p.CacheSize },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool { return p.PreloadCache > 0 },
		apply:      func(dst *BTSets, p BTSets) { dst.PreloadCache = p.PreloadCache },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool { return p.ConnectionsLimit > 0 },
		apply:      func(dst *BTSets, p BTSets) { dst.ConnectionsLimit = p.ConnectionsLimit },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool { return p.MaxConcurrentStreams > 0 || p.MaxConcurrentStreams == 0 && balancedZero(p, 64*1024*1024) },
		apply:      func(dst *BTSets, p BTSets) { dst.MaxConcurrentStreams = p.MaxConcurrentStreams },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool { return p.StreamQueueSize > 0 || p.StreamQueueSize == 0 && balancedZero(p, 64*1024*1024) },
		apply:      func(dst *BTSets, p BTSets) { dst.StreamQueueSize = p.StreamQueueSize },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool { return p.StreamQueueWaitSec > 0 },
		apply:      func(dst *BTSets, p BTSets) { dst.StreamQueueWaitSec = p.StreamQueueWaitSec },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool { return p.AdaptiveRAMinMB > 0 },
		apply:      func(dst *BTSets, p BTSets) { dst.AdaptiveRAMinMB = p.AdaptiveRAMinMB },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool { return p.AdaptiveRAMaxMB > 0 },
		apply:      func(dst *BTSets, p BTSets) { dst.AdaptiveRAMaxMB = p.AdaptiveRAMaxMB },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool { return p.WarmDiskCacheSizeMB > 0 || p.WarmDiskCacheSizeMB == 0 && balancedZero(p, 64*1024*1024) },
		apply:      func(dst *BTSets, p BTSets) { dst.WarmDiskCacheSizeMB = p.WarmDiskCacheSizeMB },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool { return p.WarmDiskCacheTTLMin > 0 },
		apply:      func(dst *BTSets, p BTSets) { dst.WarmDiskCacheTTLMin = p.WarmDiskCacheTTLMin },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool { return p.MetadataWorkers > 0 || p.MetadataWorkers == 0 && balancedZero(p, 64*1024*1024) },
		apply:      func(dst *BTSets, p BTSets) { dst.MetadataWorkers = p.MetadataWorkers },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool { return p.MetadataQueueSize > 0 || p.MetadataQueueSize == 0 && balancedZero(p, 64*1024*1024) },
		apply:      func(dst *BTSets, p BTSets) { dst.MetadataQueueSize = p.MetadataQueueSize },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool { return p.PreloadWorkers > 0 || p.PreloadWorkers == 0 && balancedZero(p, 64*1024*1024) },
		apply:      func(dst *BTSets, p BTSets) { dst.PreloadWorkers = p.PreloadWorkers },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool { return p.PreloadQueueSize > 0 || p.PreloadQueueSize == 0 && balancedZero(p, 64*1024*1024) },
		apply:      func(dst *BTSets, p BTSets) { dst.PreloadQueueSize = p.PreloadQueueSize },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool { return p.DiskSyncPolicy != "" },
		apply:      func(dst *BTSets, p BTSets) { dst.DiskSyncPolicy = p.DiskSyncPolicy },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool { return p.DiskSyncIntervalMS > 0 },
		apply:      func(dst *BTSets, p BTSets) { dst.DiskSyncIntervalMS = p.DiskSyncIntervalMS },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool { return p.DiskWriteBatchSize > 0 },
		apply:      func(dst *BTSets, p BTSets) { dst.DiskWriteBatchSize = p.DiskWriteBatchSize },
	},
}

// applyProfileFields copies non-zero fields from preset to dst, applying
// runtime-computed values for CPU-dependent fields where needed.
func applyProfileFields(dst *BTSets, preset BTSets) {
	preset = computeCPUFields(preset)
	cacheSize := preset.CacheSize

	for _, rule := range copyRules {
		if rule.shouldCopy(preset, cacheSize) {
			rule.apply(dst, preset)
		}
	}
}

func applyCoreProfilePreset(sets *BTSets, profile string) {
	preset, ok := profilePresets[profile]
	if !ok {
		preset = balancedProfile
	}

	applyProfileFields(sets, preset)
}

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

	sets.validateAndNormalize()

	if sets.UseDisk && sets.TorrentsSavePath != "" {
		resolveTorrentsSavePath(sets)
	} else if sets.TorrentsSavePath == "" {
		sets.UseDisk = false
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

// resolveTorrentsSavePath searches for a .tsc directory within the configured
// TorrentsSavePath and updates the path if found. It returns true when a valid
// path was provided (regardless of whether .tsc was found).
func resolveTorrentsSavePath(sets *BTSets) bool {
	if sets.TorrentsSavePath == "" {
		return false
	}

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

	return true
}

// validateAndNormalize applies defaults and normalization rules to BTSets fields.
// This mirrors ensureDefaults() but is tailored for incoming configuration updates.
func (s *BTSets) validateAndNormalize() {
	// Failsafe defaults
	if s.CacheSize == 0 {
		s.CacheSize = 64 * 1024 * 1024
	}

	if s.ConnectionsLimit == 0 {
		s.ConnectionsLimit = 25
	}

	if s.TorrentDisconnectTimeout == 0 {
		s.TorrentDisconnectTimeout = 30
	}

	if s.StreamQueueWaitSec <= 0 {
		s.StreamQueueWaitSec = 3
	}

	// Non-negative integer constraints
	s.ensureNonNegative("MaxConcurrentStreams", &s.MaxConcurrentStreams)
	s.ensureNonNegative("StreamQueueSize", &s.StreamQueueSize)
	s.ensureNonNegative("MetadataWorkers", &s.MetadataWorkers)
	s.ensureNonNegative("MetadataQueueSize", &s.MetadataQueueSize)
	s.ensureNonNegative("PreloadWorkers", &s.PreloadWorkers)
	s.ensureNonNegative("PreloadQueueSize", &s.PreloadQueueSize)

	// Adaptive RAM constraints
	s.ensureAdaptiveRAMDefaults()

	// Disk cache constraints
	if s.WarmDiskCacheSizeMB < 0 {
		s.WarmDiskCacheSizeMB = 0
	}

	if s.WarmDiskCacheTTLMin < 0 {
		s.WarmDiskCacheTTLMin = 0
	}

	// Disk sync policy
	s.normalizeDiskSyncPolicy()

	if s.DiskSyncIntervalMS <= 0 {
		s.DiskSyncIntervalMS = 1000
	}

	if s.DiskWriteBatchSize <= 0 {
		s.DiskWriteBatchSize = 16
	}

	// Reader and cache percentage bounds
	s.clampReaderReadAHead()
	s.clampPreloadCache()
}

// clampReaderReadAHead ensures ReaderReadAHead is within 5-100%.
func (s *BTSets) clampReaderReadAHead() {
	if s.ReaderReadAHead < 5 {
		s.ReaderReadAHead = 5
	}

	if s.ReaderReadAHead > 100 {
		s.ReaderReadAHead = 100
	}
}

// clampPreloadCache ensures PreloadCache is within 0-100%.
func (s *BTSets) clampPreloadCache() {
	if s.PreloadCache < 0 {
		s.PreloadCache = 0
	}

	if s.PreloadCache > 100 {
		s.PreloadCache = 100
	}
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
	if len(buf) == 0 {
		log.TLogln("No settings found, using defaults")
		SetDefaultConfig()

		return
	}

	loaded := new(BTSets)
	if err := json.Unmarshal(buf, loaded); err != nil {
		log.TLogln("Error unmarshal btsets:", err)
		SetDefaultConfig()

		return
	}

	loaded.ensureDefaults()

	btsetsMu.Lock()
	BTsets = loaded
	btsetsMu.Unlock()
}

// ensureDefaults applies default values and validation rules to BTSets fields.
func (s *BTSets) ensureDefaults() {
	// Core streaming settings
	if s.ReaderReadAHead < 5 {
		s.ReaderReadAHead = 5
	}

	if s.CacheSize == 0 {
		s.CacheSize = 64 * 1024 * 1024
	}

	if s.ConnectionsLimit == 0 {
		s.ConnectionsLimit = 25
	}

	if s.TorrentDisconnectTimeout == 0 {
		s.TorrentDisconnectTimeout = 30
	}

	// ResponsiveMode is critical for streaming
	s.ResponsiveMode = true

	s.CoreProfile = normalizeCoreProfile(s.CoreProfile)
	if s.StreamQueueWaitSec <= 0 {
		s.StreamQueueWaitSec = 3
	}

	s.ensureNonNegative("MaxConcurrentStreams", &s.MaxConcurrentStreams)
	s.ensureNonNegative("StreamQueueSize", &s.StreamQueueSize)
	s.ensureNonNegative("MetadataWorkers", &s.MetadataWorkers)
	s.ensureNonNegative("MetadataQueueSize", &s.MetadataQueueSize)
	s.ensureNonNegative("PreloadWorkers", &s.PreloadWorkers)
	s.ensureNonNegative("PreloadQueueSize", &s.PreloadQueueSize)

	if s.WarmDiskCacheSizeMB < 0 {
		s.WarmDiskCacheSizeMB = 0
	}

	// Adaptive RAM settings
	s.ensureAdaptiveRAMDefaults()

	// Disk cache settings
	if s.WarmDiskCacheTTLMin <= 0 {
		s.WarmDiskCacheTTLMin = 180
	}

	s.normalizeDiskSyncPolicy()
	if s.DiskSyncIntervalMS <= 0 {
		s.DiskSyncIntervalMS = 1000
	}

	if s.DiskWriteBatchSize <= 0 {
		s.DiskWriteBatchSize = 16
	}

	// TMDB defaults
	s.ensureTMDBDefaults()
}

// ensureNonNegative sets value to 0 if negative, with logging for debug.
func (s *BTSets) ensureNonNegative(name string, val *int) {
	if *val < 0 {
		*val = 0
	}
}

// ensureAdaptiveRAMDefaults sets sane defaults for adaptive RAM limits.
func (s *BTSets) ensureAdaptiveRAMDefaults() {
	if s.AdaptiveRAMinMB < 0 {
		s.AdaptiveRAMinMB = 0
	}

	if s.AdaptiveRAMaxMB < 0 {
		s.AdaptiveRAMaxMB = 0
	}

	if s.AdaptiveRAMinMB == 0 {
		s.AdaptiveRAMinMB = 4
	}

	if s.AdaptiveRAMaxMB == 0 {
		s.AdaptiveRAMaxMB = 64
	}

	if s.AdaptiveRAMinMB > s.AdaptiveRAMaxMB {
		s.AdaptiveRAMinMB = s.AdaptiveRAMaxMB
	}
}

// normalizeDiskSyncPolicy ensures DiskSyncPolicy is valid.
func (s *BTSets) normalizeDiskSyncPolicy() {
	if s.DiskSyncPolicy == "" {
		s.DiskSyncPolicy = "periodic"
	}

	policy := strings.ToLower(s.DiskSyncPolicy)

	switch policy {
	case "none", "periodic", "always":
		s.DiskSyncPolicy = policy
	default:
		s.DiskSyncPolicy = "periodic"
	}
}

// ensureTMDBDefaults sets default TMDB configuration if missing.
func (s *BTSets) ensureTMDBDefaults() {
	if s.TMDBSettings.APIURL == "" {
		s.TMDBSettings = TMDBConfig{
			APIKey:     "",
			APIURL:     "https://api.themoviedb.org",
			ImageURL:   "https://image.tmdb.org",
			ImageURLRu: "https://imagetmdb.com",
		}
	}
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
