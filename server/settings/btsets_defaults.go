package settings

import (
	"encoding/json"
	"strings"

	"server/log"
)

// validateAndNormalize applies defaults and normalization rules to BTSets fields.
// This mirrors ensureDefaults() but is tailored for incoming configuration updates.
func (s *BTSets) validateAndNormalize() {
	if s.CacheSize == 0 {
		s.CacheSize = 64 * 1024 * 1024
	}

	if s.PreloadCache == 0 {
		s.PreloadCache = 50
	}

	if !s.ResponsiveMode {
		s.ResponsiveMode = true
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

	s.ensureNonNegative("MaxConcurrentStreams", &s.MaxConcurrentStreams)
	s.ensureNonNegative("StreamQueueSize", &s.StreamQueueSize)
	s.ensureNonNegative("MetadataWorkers", &s.MetadataWorkers)
	s.ensureNonNegative("MetadataQueueSize", &s.MetadataQueueSize)
	s.ensureNonNegative("PreloadWorkers", &s.PreloadWorkers)
	s.ensureNonNegative("PreloadQueueSize", &s.PreloadQueueSize)

	s.ensureAdaptiveRAMDefaults()

	if s.WarmDiskCacheSizeMB < 0 {
		s.WarmDiskCacheSizeMB = 0
	}

	if s.WarmDiskCacheTTLMin < 0 {
		s.WarmDiskCacheTTLMin = 0
	}

	s.normalizeDiskSyncPolicy()

	if s.DiskSyncIntervalMS <= 0 {
		s.DiskSyncIntervalMS = 1000
	}

	if s.DiskWriteBatchSize <= 0 {
		s.DiskWriteBatchSize = 16
	}

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
	sets.MaxConcurrentStreams = 0
	sets.StreamQueueSize = 0
	sets.StreamQueueWaitSec = 3
	sets.AdaptiveRAMinMB = 4
	sets.AdaptiveRAMaxMB = 64
	sets.WarmDiskCacheSizeMB = 0
	sets.WarmDiskCacheTTLMin = 180
	sets.DiskSyncPolicy = "periodic"
	sets.DiskSyncIntervalMS = 1000
	sets.DiskWriteBatchSize = 16
	sets.MetadataWorkers = 0
	sets.MetadataQueueSize = 0
	sets.PreloadWorkers = 0
	sets.PreloadQueueSize = 0
	sets.ShowFSActiveTorr = true
	sets.StoreSettingsInJSON = true
	sets.TMDBSettings = TMDBConfig{
		APIKey:     "",
		APIURL:     "https://api.themoviedb.org",
		ImageURL:   "https://image.tmdb.org",
		ImageURLRu: "https://imagetmdb.com",
	}

	defaultBTsetsStore.set(sets)

	if !IsReadOnlyMode() {
		buf, err := json.Marshal(sets)
		if err != nil {
			log.TLogln("Error marshal btsets", err)

			return
		}

		tdb.Set("Settings", "BitTorr", buf)
	}

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
	defaultBTsetsStore.set(loaded)
}

// ensureDefaults applies default values and validation rules to BTSets fields.
func (s *BTSets) ensureDefaults() {
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

	s.ensureAdaptiveRAMDefaults()

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
