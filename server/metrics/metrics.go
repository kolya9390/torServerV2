// Package metrics exposes runtime metrics via expvar for /debug/vars.
package metrics

import (
	"expvar"
	"runtime"
	"sort"
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
	expvar.Publish("cache_resident_pieces", expvar.Func(func() any {
		return torrstor.SnapshotCacheStats().ResidentPieces
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
	expvar.Publish("cache_priority", expvar.Func(func() any {
		return torrstor.SnapshotCachePriorityStats()
	}))
	expvar.Publish("cache_reader_lifecycle", expvar.Func(func() any {
		return torrstor.SnapshotReaderLifecycleStats()
	}))
	expvar.Publish("request_strategy_storage", expvar.Func(func() any {
		return torrstor.SnapshotRequestStrategyCapacityDiagnostics()
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
			torrstor.SnapshotRequestStrategyCapacityDiagnostics(),
		)
	}))
	expvar.Publish("stream_health", expvar.Func(func() any {
		return torr.SnapshotStreamHealth()
	}))
	expvar.Publish("active_stream_delivery", expvar.Func(func() any {
		return torr.SnapshotStreamDelivery()
	}))
	expvar.Publish("stream_fairness", expvar.Func(func() any {
		return torr.SnapshotStreamFairness()
	}))
	expvar.Publish("stream_admission", expvar.Func(func() any {
		return torr.SnapshotStreamAdmission()
	}))
	expvar.Publish("stream_sessions", expvar.Func(func() any {
		return streamSessionDiagnosticsSnapshot(resolved.TorrentBackend)
	}))
	expvar.Publish("peer_acquisition", expvar.Func(func() any {
		return torr.SnapshotPeerAcquisitionBoost()
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
	debugCfg := sets.DebugConfig()
	policy := torr.ConnectionPolicyForSettings(sets)

	return map[string]any{
		"connections_limit":                    networkCfg.ConnectionsLimit,
		"effective_conns":                      policy.EffectiveConns,
		"peer_low_water":                       policy.PeerLowWater,
		"peer_high_water":                      policy.PeerHighWater,
		"total_half_open_conns":                policy.TotalHalfOpen,
		"tracker_budget":                       policy.TrackerBudget,
		"low_cpu_profile":                      policy.LowCPUProfile,
		"debug_established_conns_override":     policy.DebugOverride,
		"debug_total_half_open_conns_override": policy.DebugHalfOpenOverride,
		"debug_tracker_budget_override":        policy.DebugTrackerOverride,
		"debug_stable_peer_cap":                policy.DebugStablePeerCap,
		"debug_max_unverified_bytes":           torrentDebugMaxUnverifiedBytes(debugCfg),
		"dht_enabled":                          !networkCfg.DisableDHT,
		"pex_enabled":                          !networkCfg.DisablePEX,
		"tcp_enabled":                          !networkCfg.DisableTCP,
		"utp_enabled":                          !networkCfg.DisableUTP,
		"upload_enabled":                       !networkCfg.DisableUpload,
		"download_rate_limit_kbs":              networkCfg.DownloadRateLimitKB,
		"upload_rate_limit_kbs":                networkCfg.UploadRateLimitKB,
	}
}

func torrentDebugMaxUnverifiedBytes(debugCfg settings.DebugConfig) int64 {
	if !debugCfg.EnableDebug || debugCfg.MaxUnverifiedBytesMB <= 0 {
		return 0
	}

	return debugCfg.MaxUnverifiedBytesMB << 20
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

		items = append(items, torrentRuntimeItem(index, snapshot))
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

func torrentRuntimeItem(index int, snapshot torr.TorrentRuntimeMetrics) map[string]any {
	return map[string]any{
		"torrent_id":            snapshot.RuntimeID,
		"index":                 index,
		"active_peers":          snapshot.ActivePeers,
		"total_peers":           snapshot.TotalPeers,
		"pending_peers":         snapshot.PendingPeers,
		"half_open_peers":       snapshot.HalfOpenPeers,
		"connected_seeders":     snapshot.ConnectedSeeders,
		"max_established_conns": snapshot.MaxEstablished,
		"active_readers":        snapshot.ActiveReaders,
		"total_readers":         snapshot.TotalReaders,
		"idle_readers":          snapshot.IdleReaders,
		"oldest_reader_age_ms":  snapshot.OldestReaderMS,
		"newest_reader_age_ms":  snapshot.NewestReaderMS,
		"max_reader_idle_ms":    snapshot.MaxReaderIdleMS,
		"preload_active":        snapshot.PreloadActive,
		"preloaded_bytes":       snapshot.PreloadedBytes,
		"preload_target_bytes":  snapshot.PreloadTarget,
		"tracker_tiers":         snapshot.TrackerTiers,
		"trackers":              snapshot.Trackers,
		"download_speed":        snapshot.DownloadSpeed,
		"upload_speed":          snapshot.UploadSpeed,
	}
}

type streamSessionTorrentSource struct {
	torrentID          uint64
	cacheReaders       int
	totalCacheReaders  int
	idleCacheReaders   int
	oldestReaderAgeMS  int64
	newestReaderAgeMS  int64
	maxReaderIdleMS    int64
	preloadActive      bool
	preloadedBytes     int64
	preloadTargetBytes int64
}

type streamSessionTotals struct {
	cacheReaders          int
	totalCacheReaders     int
	idleCacheReaders      int
	helperReadersEstimate int
	deliveryStreams       int
	preloadActiveTorrents int
	maxReaderIdleMS       int64
	oldestReaderAgeMS     int64
	items                 []map[string]any
}

func streamSessionDiagnosticsSnapshot(backend torr.TorrentService) map[string]any {
	runtimeSnapshot := torrentRuntimeSnapshot(backend)
	admission := torr.SnapshotStreamAdmission()
	delivery := torr.SnapshotStreamDelivery()
	playbackByTorrent := make(map[uint64]int, len(admission.Streams))
	deliveryByTorrent := make(map[uint64]int, len(delivery.Streams))

	for _, stream := range admission.Streams {
		playbackByTorrent[stream.TorrentID] += stream.Readers
	}

	for _, stream := range delivery.Streams {
		deliveryByTorrent[stream.TorrentID]++
	}

	return streamSessionSnapshotFromSources(
		runtimeSnapshot,
		playbackByTorrent,
		deliveryByTorrent,
		int(admission.ActiveStreams),
		admission.ActiveUniquePlaybackTorrents,
	)
}

func streamSessionSnapshotFromSources(
	runtimeSnapshot map[string]any,
	playbackByTorrent map[uint64]int,
	deliveryByTorrent map[uint64]int,
	activePlaybackSessions int,
	activeUniquePlaybackTorrents int,
) map[string]any {
	torrents := streamSessionRuntimeSources(runtimeSnapshot)
	ensureStreamSessionSources(torrents, playbackByTorrent)
	ensureStreamSessionSources(torrents, deliveryByTorrent)

	totals := collectStreamSessionTotals(
		torrents,
		playbackByTorrent,
		deliveryByTorrent,
		sortedStreamSessionIDs(torrents),
	)

	return totals.snapshot(activePlaybackSessions, activeUniquePlaybackTorrents)
}

func ensureStreamSessionSources(sources map[uint64]streamSessionTorrentSource, counts map[uint64]int) {
	for torrentID := range counts {
		if _, ok := sources[torrentID]; !ok {
			sources[torrentID] = streamSessionTorrentSource{torrentID: torrentID}
		}
	}
}

func sortedStreamSessionIDs(torrents map[uint64]streamSessionTorrentSource) []uint64 {
	ids := make([]uint64, 0, len(torrents))
	for torrentID := range torrents {
		ids = append(ids, torrentID)
	}

	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})

	return ids
}

func collectStreamSessionTotals(
	torrents map[uint64]streamSessionTorrentSource,
	playbackByTorrent map[uint64]int,
	deliveryByTorrent map[uint64]int,
	ids []uint64,
) streamSessionTotals {
	totals := streamSessionTotals{
		items: make([]map[string]any, 0, len(ids)),
	}

	for _, torrentID := range ids {
		source := torrents[torrentID]
		playbackSessions := playbackByTorrent[torrentID]
		torrentDeliveryStreams := deliveryByTorrent[torrentID]

		totals.addTorrent(source, playbackSessions, torrentDeliveryStreams)
	}

	return totals
}

func (t *streamSessionTotals) addTorrent(
	source streamSessionTorrentSource,
	playbackSessions int,
	deliveryStreams int,
) {
	helperReaders := maxIntMetric(source.cacheReaders-playbackSessions, 0)
	t.cacheReaders += source.cacheReaders
	t.totalCacheReaders += source.totalCacheReaders
	t.idleCacheReaders += source.idleCacheReaders
	t.helperReadersEstimate += helperReaders
	t.deliveryStreams += deliveryStreams

	if source.preloadActive {
		t.preloadActiveTorrents++
	}

	if source.maxReaderIdleMS > t.maxReaderIdleMS {
		t.maxReaderIdleMS = source.maxReaderIdleMS
	}

	if source.oldestReaderAgeMS > t.oldestReaderAgeMS {
		t.oldestReaderAgeMS = source.oldestReaderAgeMS
	}

	t.items = append(t.items, streamSessionTorrentItem(source, playbackSessions, deliveryStreams, helperReaders))
}

func streamSessionTorrentItem(
	source streamSessionTorrentSource,
	playbackSessions int,
	deliveryStreams int,
	helperReaders int,
) map[string]any {
	return map[string]any{
		"torrent_id":              source.torrentID,
		"playback_sessions":       playbackSessions,
		"delivery_streams":        deliveryStreams,
		"cache_readers":           source.cacheReaders,
		"total_cache_readers":     source.totalCacheReaders,
		"idle_cache_readers":      source.idleCacheReaders,
		"helper_readers_estimate": helperReaders,
		"oldest_reader_age_ms":    source.oldestReaderAgeMS,
		"newest_reader_age_ms":    source.newestReaderAgeMS,
		"max_reader_idle_ms":      source.maxReaderIdleMS,
		"preload_active":          source.preloadActive,
		"preloaded_bytes":         source.preloadedBytes,
		"preload_target_bytes":    source.preloadTargetBytes,
		"classification":          streamSessionClassification(source, playbackSessions, deliveryStreams),
	}
}

func (t *streamSessionTotals) snapshot(activePlaybackSessions, activeUniquePlaybackTorrents int) map[string]any {
	return map[string]any{
		"active_playback_sessions":        activePlaybackSessions,
		"active_unique_playback_torrents": activeUniquePlaybackTorrents,
		"active_delivery_streams":         t.deliveryStreams,
		"active_cache_readers":            t.cacheReaders,
		"total_cache_readers":             t.totalCacheReaders,
		"idle_cache_readers":              t.idleCacheReaders,
		"helper_readers_estimate":         t.helperReadersEstimate,
		"preload_active_torrents":         t.preloadActiveTorrents,
		"oldest_reader_age_ms":            t.oldestReaderAgeMS,
		"max_reader_idle_ms":              t.maxReaderIdleMS,
		"torrents":                        t.items,
		"interpretation": streamSessionInterpretation(
			t.helperReadersEstimate,
			t.deliveryStreams,
			t.cacheReaders,
		),
	}
}

func streamSessionRuntimeSources(runtimeSnapshot map[string]any) map[uint64]streamSessionTorrentSource {
	items, ok := runtimeSnapshot["torrents"].([]map[string]any)
	if !ok {
		return map[uint64]streamSessionTorrentSource{}
	}

	sources := make(map[uint64]streamSessionTorrentSource, len(items))

	for _, item := range items {
		torrentID := uint64Metric(item, "torrent_id")
		if torrentID == 0 {
			continue
		}

		sources[torrentID] = streamSessionTorrentSource{
			torrentID:          torrentID,
			cacheReaders:       intMetric(item, "active_readers"),
			totalCacheReaders:  intMetric(item, "total_readers"),
			idleCacheReaders:   intMetric(item, "idle_readers"),
			oldestReaderAgeMS:  int64Metric(item, "oldest_reader_age_ms"),
			newestReaderAgeMS:  int64Metric(item, "newest_reader_age_ms"),
			maxReaderIdleMS:    int64Metric(item, "max_reader_idle_ms"),
			preloadActive:      boolMetric(item, "preload_active"),
			preloadedBytes:     int64Metric(item, "preloaded_bytes"),
			preloadTargetBytes: int64Metric(item, "preload_target_bytes"),
		}
	}

	return sources
}

func streamSessionClassification(source streamSessionTorrentSource, playbackSessions, deliveryStreams int) string {
	switch {
	case playbackSessions == 0 && source.cacheReaders > 0:
		return "cache_without_playback"
	case playbackSessions > 0 && source.cacheReaders > playbackSessions:
		return "extra_cache_readers"
	case deliveryStreams > playbackSessions:
		return "delivery_without_admission"
	case source.preloadActive && source.cacheReaders == 0:
		return "preload_only"
	case playbackSessions > 0 && source.cacheReaders == 0:
		return "playback_without_cache_reader"
	case playbackSessions > 0:
		return "playback_aligned"
	default:
		return "idle"
	}
}

func streamSessionInterpretation(helperReaders, deliveryStreams, cacheReaders int) string {
	switch {
	case helperReaders > 0:
		return "cache reader pressure is higher than playback session demand; inspect range/reconnect helper readers"
	case deliveryStreams > cacheReaders:
		return "delivery streams exceed active cache readers; inspect startup, close, and instrumentation ordering"
	default:
		return "playback session demand and cache reader pressure are aligned"
	}
}

func requestStrategyPressureSnapshot(
	runtime map[string]any,
	cacheStats torrstor.CacheStats,
	capacity torrstor.RequestStrategyCapacityDiagnostics,
) map[string]any {
	activePeers := intMetric(runtime, "active_peers")
	totalPeers := intMetric(runtime, "total_peers")
	activeReaders := intMetric(runtime, "active_readers")
	connectedSeeders := intMetric(runtime, "connected_seeders")
	downloadSpeed := aggregateTorrentRuntimeInt(runtime, "download_speed")
	cacheFillPct := percent(cacheStats.LogicalFilledBytes, cacheStats.ConfiguredCapacityBytes)
	cacheOverheadPct := percent(cacheStats.LogicalOverheadBytes, cacheStats.ConfiguredCapacityBytes)
	score := requestStrategyPressureScore(activePeers, activeReaders, cacheStats.ResidentPieces)
	level := requestStrategyPressureLevel(activePeers, activeReaders, cacheStats.ResidentPieces)
	interpretation := requestStrategyPressureInterpretation(level)

	if capacity.UncappedCaches > 0 || capacity.InvalidCaches > 0 {
		level = "fault"
		interpretation = capacity.Interpretation
	}

	return map[string]any{
		"level":                      level,
		"score":                      score,
		"active_peers":               activePeers,
		"total_peers":                totalPeers,
		"connected_seeders":          connectedSeeders,
		"active_readers":             activeReaders,
		"download_speed":             downloadSpeed,
		"cache_pieces_count":         cacheStats.PiecesCount,
		"cache_resident_pieces":      cacheStats.ResidentPieces,
		"cache_in_memory_chunks":     cacheStats.InMemoryChunks,
		"cache_logical_filled_bytes": cacheStats.LogicalFilledBytes,
		"cache_fill_percent":         cacheFillPct,
		"cache_overhead_percent":     cacheOverheadPct,
		"cache_misses":               cacheStats.Misses,
		"storage_capacity_status":    capacity.Status,
		"storage_capped_caches":      capacity.CappedCaches,
		"storage_uncapped_caches":    capacity.UncappedCaches,
		"storage_invalid_caches":     capacity.InvalidCaches,
		"bounded_requestable_pieces": capacity.BoundedRequestablePieceEstimate,
		"interpretation":             interpretation,
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

func int64Metric(metrics map[string]any, key string) int64 {
	if metrics == nil {
		return 0
	}

	switch value := metrics[key].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case uint64:
		return int64(value)
	case float64:
		return int64(value)
	default:
		return 0
	}
}

func uint64Metric(metrics map[string]any, key string) uint64 {
	if metrics == nil {
		return 0
	}

	switch value := metrics[key].(type) {
	case int:
		if value < 0 {
			return 0
		}

		return uint64(value)
	case int64:
		if value < 0 {
			return 0
		}

		return uint64(value)
	case uint64:
		return value
	case float64:
		if value < 0 {
			return 0
		}

		return uint64(value)
	default:
		return 0
	}
}

func boolMetric(metrics map[string]any, key string) bool {
	if metrics == nil {
		return false
	}

	value, ok := metrics[key].(bool)

	return ok && value
}

func maxIntMetric(left, right int) int {
	if left > right {
		return left
	}

	return right
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
