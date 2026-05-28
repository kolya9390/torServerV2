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

var (
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
		registerCacheMetrics()
		registerRuntimeMetrics(resolved)

		if shouldStartRuntimeUpdater(resolved.SettingsProvider) {
			go updateRuntimeMetrics(resolved)
		}
	})
}

func registerCacheMetrics() {
	expvar.Publish("cache_hits", expvar.Func(func() any {
		return torrstor.SnapshotCacheStats().Hits
	}))
	expvar.Publish("cache_misses", expvar.Func(func() any {
		return torrstor.SnapshotCacheStats().Misses
	}))
	expvar.Publish("cache_active", expvar.Func(func() any {
		return torrstor.SnapshotCacheStats().ActiveCaches
	}))
	expvar.Publish("cache_active_readers", expvar.Func(func() any {
		return torrstor.SnapshotCacheStats().ActiveReaders
	}))
	expvar.Publish("cache_logical_filled_bytes", expvar.Func(func() any {
		return torrstor.SnapshotCacheStats().LogicalFilledBytes
	}))
	expvar.Publish("cache_configured_capacity_bytes", expvar.Func(func() any {
		return torrstor.SnapshotCacheStats().ConfiguredCapacityBytes
	}))
	expvar.Publish("cache_logical_overhead_bytes", expvar.Func(func() any {
		return torrstor.SnapshotCacheStats().LogicalOverheadBytes
	}))
	expvar.Publish("cache_pieces_count", expvar.Func(func() any {
		return torrstor.SnapshotCacheStats().PiecesCount
	}))
	expvar.Publish("cache_in_memory_chunks", expvar.Func(func() any {
		return torrstor.SnapshotCacheStats().InMemoryChunks
	}))
	expvar.Publish("cache_reusable_chunks", expvar.Func(func() any {
		return torrstor.SnapshotCacheStats().ReusableChunks
	}))
	expvar.Publish("cache_reusable_bytes", expvar.Func(func() any {
		return torrstor.SnapshotCacheStats().ReusableBytes
	}))
	expvar.Publish("cache_cleanup_runs", expvar.Func(func() any {
		return torrstor.SnapshotCacheStats().CleanupRuns
	}))
	expvar.Publish("cache_cleaned_bytes", expvar.Func(func() any {
		return torrstor.SnapshotCacheStats().CleanedBytes
	}))
	expvar.Publish("cache_memory", expvar.Func(func() any {
		return torrstor.SnapshotCacheStats()
	}))
}

func registerRuntimeMetrics(resolved Deps) {
	expvar.Publish("active_streams", expvar.Func(func() any {
		return torr.GetActiveStreams()
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
	expvar.Publish("torrent_connection_policy", expvar.Func(func() any {
		return torrentConnectionPolicySnapshot(resolved.SettingsProvider.Get())
	}))
	expvar.Publish("torrent_runtime", expvar.Func(func() any {
		return torrentRuntimeSnapshot(resolved.TorrentBackend)
	}))
	expvar.Publish("request_strategy_pressure", expvar.Func(func() any {
		return requestStrategyPressureSnapshot(
			torrentRuntimeSnapshot(resolved.TorrentBackend),
			torrstor.SnapshotCacheStats(),
		)
	}))
	expvar.Publish("stream_health", expvar.Func(func() any {
		return torr.SnapshotStreamHealth()
	}))
}

func shouldStartRuntimeUpdater(provider settings.SettingsProvider) bool {
	if provider == nil {
		return false
	}

	sets := provider.Get()
	if sets == nil {
		return false
	}

	return sets.DebugConfig().EnableDebug
}

func torrentConnectionPolicySnapshot(sets *settings.BTSets) map[string]any {
	if sets == nil {
		sets = &settings.BTSets{}
	}

	networkCfg := sets.NetworkConfig()
	policy := torr.ConnectionPolicyForSettings(sets)

	return map[string]any{
		"connections_limit":       networkCfg.ConnectionsLimit,
		"effective_conns":         policy.EffectiveConns,
		"peer_low_water":          policy.PeerLowWater,
		"peer_high_water":         policy.PeerHighWater,
		"tracker_budget":          policy.TrackerBudget,
		"low_cpu_profile":         policy.LowCPUProfile,
		"dht_enabled":             !networkCfg.DisableDHT,
		"pex_enabled":             !networkCfg.DisablePEX,
		"tcp_enabled":             !networkCfg.DisableTCP,
		"utp_enabled":             !networkCfg.DisableUTP,
		"upload_enabled":          !networkCfg.DisableUpload,
		"download_rate_limit_kbs": networkCfg.DownloadRateLimitKB,
		"upload_rate_limit_kbs":   networkCfg.UploadRateLimitKB,
	}
}

func torrentRuntimeSnapshot(backend torr.TorrentService) map[string]any {
	if backend == nil {
		backend = torr.NewNoopTorrentService()
	}

	torrents := backend.ListTorrents()
	items := make([]map[string]any, 0, len(torrents))

	var activePeers, totalPeers, pendingPeers, halfOpenPeers, connectedSeeders, activeReaders int

	var trackerTiers, trackers int

	for index, torrent := range torrents {
		if torrent == nil {
			continue
		}

		snapshot, ok := torrent.RuntimeMetricsSnapshot()
		if !ok {
			continue
		}

		activePeers += snapshot.ActivePeers
		totalPeers += snapshot.TotalPeers
		pendingPeers += snapshot.PendingPeers
		halfOpenPeers += snapshot.HalfOpenPeers
		connectedSeeders += snapshot.ConnectedSeeders
		activeReaders += snapshot.ActiveReaders
		trackerTiers += snapshot.TrackerTiers
		trackers += snapshot.Trackers

		items = append(items, map[string]any{
			"index":             index,
			"active_peers":      snapshot.ActivePeers,
			"total_peers":       snapshot.TotalPeers,
			"pending_peers":     snapshot.PendingPeers,
			"half_open_peers":   snapshot.HalfOpenPeers,
			"connected_seeders": snapshot.ConnectedSeeders,
			"active_readers":    snapshot.ActiveReaders,
			"tracker_tiers":     snapshot.TrackerTiers,
			"trackers":          snapshot.Trackers,
			"download_speed":    snapshot.DownloadSpeed,
			"upload_speed":      snapshot.UploadSpeed,
		})
	}

	return map[string]any{
		"torrents":          items,
		"active_peers":      activePeers,
		"total_peers":       totalPeers,
		"pending_peers":     pendingPeers,
		"half_open_peers":   halfOpenPeers,
		"connected_seeders": connectedSeeders,
		"active_readers":    activeReaders,
		"tracker_tiers":     trackerTiers,
		"trackers":          trackers,
	}
}

func requestStrategyPressureSnapshot(runtime map[string]any, cacheStats torrstor.CacheStats) map[string]any {
	activePeers := intMetric(runtime, "active_peers")
	totalPeers := intMetric(runtime, "total_peers")
	activeReaders := intMetric(runtime, "active_readers")
	connectedSeeders := intMetric(runtime, "connected_seeders")
	downloadSpeed := aggregateTorrentRuntimeInt(runtime, "download_speed")
	cacheFillPct := percent(cacheStats.LogicalFilledBytes, cacheStats.ConfiguredCapacityBytes)
	cacheOverheadPct := percent(cacheStats.LogicalOverheadBytes, cacheStats.ConfiguredCapacityBytes)
	score := requestStrategyPressureScore(activePeers, activeReaders, cacheStats.PiecesCount)
	level := requestStrategyPressureLevel(activePeers, activeReaders, cacheStats.PiecesCount)

	return map[string]any{
		"level":                      level,
		"score":                      score,
		"active_peers":               activePeers,
		"total_peers":                totalPeers,
		"connected_seeders":          connectedSeeders,
		"active_readers":             activeReaders,
		"download_speed":             downloadSpeed,
		"cache_pieces_count":         cacheStats.PiecesCount,
		"cache_in_memory_chunks":     cacheStats.InMemoryChunks,
		"cache_logical_filled_bytes": cacheStats.LogicalFilledBytes,
		"cache_fill_percent":         cacheFillPct,
		"cache_overhead_percent":     cacheOverheadPct,
		"cache_misses":               cacheStats.Misses,
		"interpretation":             requestStrategyPressureInterpretation(level),
	}
}

func requestStrategyPressureScore(activePeers, activeReaders int, cachePieces int64) int64 {
	if activePeers <= 0 || activeReaders <= 0 || cachePieces <= 0 {
		return 0
	}

	return int64(activePeers) * int64(activeReaders) * cachePieces
}

func requestStrategyPressureLevel(activePeers, activeReaders int, cachePieces int64) string {
	score := requestStrategyPressureScore(activePeers, activeReaders, cachePieces)

	switch {
	case score >= 2_500_000 || activePeers >= 80 || activeReaders >= 3:
		return "high"
	case score >= 500_000 || activePeers >= 40 || activeReaders >= 2:
		return "medium"
	default:
		return "low"
	}
}

func requestStrategyPressureInterpretation(level string) string {
	switch level {
	case "high":
		return "request strategy CPU may be amplified by peers, readers, and active cache pieces"
	case "medium":
		return "watch request strategy CPU if playback latency or cache misses increase"
	default:
		return "request strategy pressure is expected to be low"
	}
}

func percent(value, total int64) int64 {
	if total <= 0 {
		return 0
	}

	return value * 100 / total
}

func intMetric(metrics map[string]any, key string) int {
	if metrics == nil {
		return 0
	}

	switch value := metrics[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func aggregateTorrentRuntimeInt(runtime map[string]any, key string) int64 {
	torrents, ok := runtime["torrents"].([]map[string]any)
	if !ok {
		return 0
	}

	var total int64

	for _, item := range torrents {
		total += int64(intMetric(item, key))
	}

	return total
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
