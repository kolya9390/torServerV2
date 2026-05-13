package settings

import (
	"runtime"
	"strings"
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
	CacheSize:            32 * 1024 * 1024,
	PreloadCache:         25,
	ConnectionsLimit:     12,
	MaxConcurrentStreams: 1,
	StreamQueueSize:      2,
	StreamQueueWaitSec:   2,
	AdaptiveRAMinMB:      2,
	AdaptiveRAMaxMB:      16,
	WarmDiskCacheSizeMB:  256,
	WarmDiskCacheTTLMin:  60,
	MetadataWorkers:      1,
	MetadataQueueSize:    64,
	PreloadWorkers:       1,
	PreloadQueueSize:     8,
	DiskSyncPolicy:       "periodic",
	DiskSyncIntervalMS:   800,
	DiskWriteBatchSize:   8,
}

// balancedProfile defines the default balanced preset.
var balancedProfile = BTSets{
	CacheSize:            64 * 1024 * 1024,
	PreloadCache:         50,
	ConnectionsLimit:     25,
	MaxConcurrentStreams: 0,
	StreamQueueSize:      0,
	StreamQueueWaitSec:   3,
	AdaptiveRAMinMB:      4,
	AdaptiveRAMaxMB:      64,
	WarmDiskCacheSizeMB:  0,
	WarmDiskCacheTTLMin:  180,
	MetadataWorkers:      0,
	MetadataQueueSize:    0,
	PreloadWorkers:       0,
	PreloadQueueSize:     0,
	DiskSyncPolicy:       "periodic",
	DiskSyncIntervalMS:   1000,
	DiskWriteBatchSize:   16,
}

// highThroughputProfile defines a preset optimized for high-performance systems.
var highThroughputProfile = BTSets{
	CacheSize:            256 * 1024 * 1024,
	PreloadCache:         70,
	ConnectionsLimit:     0, // computed at apply time
	MaxConcurrentStreams: 0, // computed at apply time
	StreamQueueSize:      0, // computed at apply time
	StreamQueueWaitSec:   4,
	AdaptiveRAMinMB:      8,
	AdaptiveRAMaxMB:      128,
	WarmDiskCacheSizeMB:  4096,
	WarmDiskCacheTTLMin:  360,
	MetadataWorkers:      0, // computed at apply time
	MetadataQueueSize:    512,
	PreloadWorkers:       0, // computed at apply time
	PreloadQueueSize:     64,
	DiskSyncPolicy:       "periodic",
	DiskSyncIntervalMS:   1500,
	DiskWriteBatchSize:   32,
}

// nasProfile defines a preset optimized for NAS devices.
var nasProfile = BTSets{
	CacheSize:            128 * 1024 * 1024,
	PreloadCache:         55,
	ConnectionsLimit:     0, // computed at apply time
	MaxConcurrentStreams: 0, // computed at apply time
	StreamQueueSize:      0, // computed at apply time
	StreamQueueWaitSec:   4,
	AdaptiveRAMinMB:      4,
	AdaptiveRAMaxMB:      64,
	WarmDiskCacheSizeMB:  8192,
	WarmDiskCacheTTLMin:  720,
	MetadataWorkers:      0, // computed at apply time
	MetadataQueueSize:    256,
	PreloadWorkers:       1,
	PreloadQueueSize:     32,
	DiskSyncPolicy:       "periodic",
	DiskSyncIntervalMS:   2500,
	DiskWriteBatchSize:   24,
}

// profilePresets maps profile names to their preset configurations.
var profilePresets = map[string]BTSets{
	"low-end":         lowEndProfile,
	"balanced":        balancedProfile,
	"high-throughput": highThroughputProfile,
	"nas":             nasProfile,
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
		shouldCopy: func(p BTSets, _ int64) bool {
			return p.MaxConcurrentStreams > 0 || p.MaxConcurrentStreams == 0 && balancedZero(p, 64*1024*1024)
		},
		apply: func(dst *BTSets, p BTSets) { dst.MaxConcurrentStreams = p.MaxConcurrentStreams },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool {
			return p.StreamQueueSize > 0 || p.StreamQueueSize == 0 && balancedZero(p, 64*1024*1024)
		},
		apply: func(dst *BTSets, p BTSets) { dst.StreamQueueSize = p.StreamQueueSize },
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
		shouldCopy: func(p BTSets, _ int64) bool {
			return p.WarmDiskCacheSizeMB > 0 || p.WarmDiskCacheSizeMB == 0 && balancedZero(p, 64*1024*1024)
		},
		apply: func(dst *BTSets, p BTSets) { dst.WarmDiskCacheSizeMB = p.WarmDiskCacheSizeMB },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool { return p.WarmDiskCacheTTLMin > 0 },
		apply:      func(dst *BTSets, p BTSets) { dst.WarmDiskCacheTTLMin = p.WarmDiskCacheTTLMin },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool {
			return p.MetadataWorkers > 0 || p.MetadataWorkers == 0 && balancedZero(p, 64*1024*1024)
		},
		apply: func(dst *BTSets, p BTSets) { dst.MetadataWorkers = p.MetadataWorkers },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool {
			return p.MetadataQueueSize > 0 || p.MetadataQueueSize == 0 && balancedZero(p, 64*1024*1024)
		},
		apply: func(dst *BTSets, p BTSets) { dst.MetadataQueueSize = p.MetadataQueueSize },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool {
			return p.PreloadWorkers > 0 || p.PreloadWorkers == 0 && balancedZero(p, 64*1024*1024)
		},
		apply: func(dst *BTSets, p BTSets) { dst.PreloadWorkers = p.PreloadWorkers },
	},
	{
		shouldCopy: func(p BTSets, _ int64) bool {
			return p.PreloadQueueSize > 0 || p.PreloadQueueSize == 0 && balancedZero(p, 64*1024*1024)
		},
		apply: func(dst *BTSets, p BTSets) { dst.PreloadQueueSize = p.PreloadQueueSize },
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
