// Package metrics exposes runtime metrics via expvar for /debug/vars.
package metrics

import (
	"expvar"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"server/settings"
	"server/torr"
	"server/torr/storage/torrstor"
)

type Deps struct {
	SettingsProvider settings.SettingsProvider
	TorrentBackend   torr.TorrentService
}

func init() {
	// Register callback for cache metrics recording
	torrstor.CacheMetricsRecorder = reportCacheMetrics
}

// reportCacheMetrics is called by Cache when cleanPieces() runs.
func reportCacheMetrics(hits, misses uint64) {
	cacheHits.Store(hits)
	cacheMisses.Store(misses)
}

var (
	cacheHits      atomic.Uint64
	cacheMisses    atomic.Uint64
	peersConnected atomic.Int64
	downloadBytes  atomic.Int64
	uploadBytes    atomic.Int64
	torrentsActive atomic.Int64

	metricsOnce sync.Once
	defaultDeps = Deps{
		SettingsProvider: settings.NewNoopSettingsProvider(),
		TorrentBackend:   torr.NewNoopTorrentService(),
	}
)

// Init registers metric collectors with expvar.
func Init() {
	InitWithDeps(defaultDeps)
}

func InitWithDeps(deps Deps) {
	resolved := resolveMetricsDeps(deps)

	metricsOnce.Do(func() {
		expvar.Publish("active_streams", expvar.Func(func() any {
			return torr.GetActiveStreams()
		}))
		expvar.Publish("cache_hits", expvar.Func(func() any {
			return cacheHits.Load()
		}))
		expvar.Publish("cache_misses", expvar.Func(func() any {
			return cacheMisses.Load()
		}))
		expvar.Publish("peers_connected", expvar.Func(func() any {
			return peersConnected.Load()
		}))
		expvar.Publish("download_bytes", expvar.Func(func() any {
			return downloadBytes.Load()
		}))
		expvar.Publish("upload_bytes", expvar.Func(func() any {
			return uploadBytes.Load()
		}))
		expvar.Publish("torrents_active", expvar.Func(func() any {
			return torrentsActive.Load()
		}))
		expvar.Publish("goroutines", expvar.Func(func() any {
			return runtime.NumGoroutine()
		}))
		expvar.Publish("heap_alloc_bytes", expvar.Func(func() any {
			var m runtime.MemStats

			runtime.ReadMemStats(&m)

			return m.Alloc
		}))
		expvar.Publish("heap_total_alloc_bytes", expvar.Func(func() any {
			var m runtime.MemStats

			runtime.ReadMemStats(&m)

			return m.TotalAlloc
		}))
		expvar.Publish("cache_config_size_mb", expvar.Func(func() any {
			return resolved.SettingsProvider.Get().CacheConfig().SizeBytes / (1024 * 1024)
		}))
		expvar.Publish("responsive_mode", expvar.Func(func() any {
			return resolved.SettingsProvider.Get().StreamConfig().ResponsiveMode
		}))

		// Periodic updater goroutine
		go updateRuntimeMetrics(resolved)
	})
}

func resolveMetricsDeps(deps Deps) Deps {
	if deps.SettingsProvider == nil {
		deps.SettingsProvider = settings.NewNoopSettingsProvider()
	}

	if deps.TorrentBackend == nil {
		deps.TorrentBackend = torr.NewNoopTorrentService()
	}

	return deps
}

func updateRuntimeMetrics(deps Deps) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		torrents := deps.TorrentBackend.ListTorrents()
		torrentsActive.Store(int64(len(torrents)))

		var totalPeers int64

		var totalDownload, totalUpload int64

		for _, t := range torrents {
			activePeers, downloadSpeed, uploadSpeed, ok := t.RuntimeSnapshot()
			if ok {
				totalPeers += int64(activePeers)
				totalDownload += downloadSpeed
				totalUpload += uploadSpeed
			}
		}

		peersConnected.Store(totalPeers)
		downloadBytes.Store(totalDownload)
		uploadBytes.Store(totalUpload)
	}
}

// IncCacheHits increments cache hit counter.
func IncCacheHits() {
	cacheHits.Add(1)
}

// IncCacheMisses increments cache miss counter.
func IncCacheMisses() {
	cacheMisses.Add(1)
}
