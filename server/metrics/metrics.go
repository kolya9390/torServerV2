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
	activeStreams  atomic.Int64
	cacheHits      atomic.Uint64
	cacheMisses    atomic.Uint64
	peersConnected atomic.Int64
	downloadBytes  atomic.Int64
	uploadBytes    atomic.Int64
	torrentsActive atomic.Int64

	metricsOnce sync.Once
)

// Init registers metric collectors with expvar.
func Init() {
	metricsOnce.Do(func() {
		expvar.Publish("active_streams", expvar.Func(func() any {
			return activeStreams.Load()
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
			if settings.BTsets == nil {
				return 0
			}

			return settings.BTsets.CacheSize / (1024 * 1024)
		}))
		expvar.Publish("responsive_mode", expvar.Func(func() any {
			if settings.BTsets == nil {
				return false
			}

			return settings.BTsets.ResponsiveMode
		}))

		// Periodic updater goroutine
		go updateRuntimeMetrics()
	})
}

func updateRuntimeMetrics() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		bts := torr.GetBTServer()
		if bts == nil {
			continue
		}

		torrents := bts.ListTorrents()
		torrentsActive.Store(int64(len(torrents)))

		var totalPeers int64

		var totalDownload, totalUpload int64

		for _, t := range torrents {
			st := t.Status()
			if st != nil {
				totalPeers += int64(st.ActivePeers)
				totalDownload += int64(st.DownloadSpeed)
				totalUpload += int64(st.UploadSpeed)
			}
		}

		peersConnected.Store(totalPeers)
		downloadBytes.Store(totalDownload)
		uploadBytes.Store(totalUpload)
	}
}

// IncActiveStreams increments active stream counter.
func IncActiveStreams() {
	activeStreams.Add(1)
}

// DecActiveStreams decrements active stream counter.
func DecActiveStreams() {
	activeStreams.Add(-1)
}

// IncCacheHits increments cache hit counter.
func IncCacheHits() {
	cacheHits.Add(1)
}

// IncCacheMisses increments cache miss counter.
func IncCacheMisses() {
	cacheMisses.Add(1)
}
