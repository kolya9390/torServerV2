package torrstor

import (
	"sync/atomic"
	"time"
)

// CacheStats is an aggregate, privacy-safe view of cache residency and churn.
// It intentionally avoids per-torrent labels to keep /debug/vars bounded and
// to avoid exposing torrent links or identifiers through operational metrics.
type CacheStats struct {
	ActiveCaches            int64  `json:"active_caches"`
	ActiveReaders           int64  `json:"active_readers"`
	LogicalFilledBytes      int64  `json:"logical_filled_bytes"`
	ConfiguredCapacityBytes int64  `json:"configured_capacity_bytes"`
	LogicalOverheadBytes    int64  `json:"logical_overhead_bytes"`
	PiecesCount             int64  `json:"pieces_count"`
	ResidentPieces          int64  `json:"resident_pieces"`
	InMemoryChunks          int64  `json:"in_memory_chunks"`
	ReusableChunks          int64  `json:"reusable_chunks"`
	ReusableBytes           int64  `json:"reusable_bytes"`
	CleanupRuns             uint64 `json:"cleanup_runs"`
	CleanedBytes            uint64 `json:"cleaned_bytes"`
	Hits                    uint64 `json:"hits"`
	Misses                  uint64 `json:"misses"`
}

type CachePriorityStats struct {
	UpdatesTotal           uint64 `json:"updates_total"`
	DesiredPiecesTotal     uint64 `json:"desired_pieces_total"`
	BudgetLimitedTotal     uint64 `json:"budget_limited_total"`
	ClearedPiecesTotal     uint64 `json:"cleared_pieces_total"`
	SetPiecesTotal         uint64 `json:"set_pieces_total"`
	NoopPiecesTotal        uint64 `json:"noop_pieces_total"`
	TrackedPieces          int64  `json:"tracked_pieces"`
	LastUpdateUnixMS       int64  `json:"last_update_unix_ms"`
	RetentionExpandedTotal uint64 `json:"retention_expanded_total"`
	RetentionClampedTotal  uint64 `json:"retention_clamped_total"`
}

// ReaderLifecycleStats is an aggregate, privacy-safe view of reader idle
// demotion/reactivation transitions.
type ReaderLifecycleStats struct {
	IdleDemotionsTotal          uint64 `json:"idle_demotions_total"`
	ReactivationsTotal          uint64 `json:"reactivations_total"`
	ReactivationsAfterIdleTotal uint64 `json:"reactivations_after_idle_total"`
	DemotionIdleMSTotal         uint64 `json:"demotion_idle_ms_total"`
	DemotionIdleMSMax           int64  `json:"demotion_idle_ms_max"`
	ReactivationIdleMSTotal     uint64 `json:"reactivation_idle_ms_total"`
	ReactivationIdleMSMax       int64  `json:"reactivation_idle_ms_max"`
}

type cacheStatsCounters struct {
	activeCaches            atomic.Int64
	activeReaders           atomic.Int64
	logicalFilledBytes      atomic.Int64
	configuredCapacityBytes atomic.Int64
	piecesCount             atomic.Int64
	residentPieces          atomic.Int64
	inMemoryChunks          atomic.Int64
	cleanupRuns             atomic.Uint64
	cleanedBytes            atomic.Uint64
	hits                    atomic.Uint64
	misses                  atomic.Uint64
	priorityUpdates         atomic.Uint64
	priorityDesiredPieces   atomic.Uint64
	priorityBudgetLimited   atomic.Uint64
	priorityClearedPieces   atomic.Uint64
	prioritySetPieces       atomic.Uint64
	priorityNoopPieces      atomic.Uint64
	priorityTrackedPieces   atomic.Int64
	priorityLastUpdateMS    atomic.Int64
	retentionExpanded       atomic.Uint64
	retentionClamped        atomic.Uint64

	readerIdleDemotions           atomic.Uint64
	readerReactivations           atomic.Uint64
	readerReactivationsAfterIdle  atomic.Uint64
	readerDemotionIdleMSTotal     atomic.Uint64
	readerDemotionIdleMSMax       atomic.Int64
	readerReactivationIdleMSTotal atomic.Uint64
	readerReactivationIdleMSMax   atomic.Int64
}

var globalCacheStats cacheStatsCounters

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}

	return b
}

// SnapshotCacheStats returns aggregate cache metrics using atomic loads only.
func SnapshotCacheStats() CacheStats {
	logicalFilled := globalCacheStats.logicalFilledBytes.Load()
	configuredCapacity := globalCacheStats.configuredCapacityBytes.Load()
	reusableChunks := reusableMemPieceChunks()

	return CacheStats{
		ActiveCaches:            globalCacheStats.activeCaches.Load(),
		ActiveReaders:           globalCacheStats.activeReaders.Load(),
		LogicalFilledBytes:      logicalFilled,
		ConfiguredCapacityBytes: configuredCapacity,
		LogicalOverheadBytes:    maxInt64(logicalFilled-configuredCapacity, 0),
		PiecesCount:             globalCacheStats.piecesCount.Load(),
		ResidentPieces:          globalCacheStats.residentPieces.Load(),
		InMemoryChunks:          globalCacheStats.inMemoryChunks.Load(),
		ReusableChunks:          reusableChunks,
		ReusableBytes:           reusableChunks * memPieceChunkSize,
		CleanupRuns:             globalCacheStats.cleanupRuns.Load(),
		CleanedBytes:            globalCacheStats.cleanedBytes.Load(),
		Hits:                    globalCacheStats.hits.Load(),
		Misses:                  globalCacheStats.misses.Load(),
	}
}

func SnapshotCachePriorityStats() CachePriorityStats {
	return CachePriorityStats{
		UpdatesTotal:           globalCacheStats.priorityUpdates.Load(),
		DesiredPiecesTotal:     globalCacheStats.priorityDesiredPieces.Load(),
		BudgetLimitedTotal:     globalCacheStats.priorityBudgetLimited.Load(),
		ClearedPiecesTotal:     globalCacheStats.priorityClearedPieces.Load(),
		SetPiecesTotal:         globalCacheStats.prioritySetPieces.Load(),
		NoopPiecesTotal:        globalCacheStats.priorityNoopPieces.Load(),
		TrackedPieces:          globalCacheStats.priorityTrackedPieces.Load(),
		LastUpdateUnixMS:       globalCacheStats.priorityLastUpdateMS.Load(),
		RetentionExpandedTotal: globalCacheStats.retentionExpanded.Load(),
		RetentionClampedTotal:  globalCacheStats.retentionClamped.Load(),
	}
}

func SnapshotReaderLifecycleStats() ReaderLifecycleStats {
	return ReaderLifecycleStats{
		IdleDemotionsTotal:          globalCacheStats.readerIdleDemotions.Load(),
		ReactivationsTotal:          globalCacheStats.readerReactivations.Load(),
		ReactivationsAfterIdleTotal: globalCacheStats.readerReactivationsAfterIdle.Load(),
		DemotionIdleMSTotal:         globalCacheStats.readerDemotionIdleMSTotal.Load(),
		DemotionIdleMSMax:           globalCacheStats.readerDemotionIdleMSMax.Load(),
		ReactivationIdleMSTotal:     globalCacheStats.readerReactivationIdleMSTotal.Load(),
		ReactivationIdleMSMax:       globalCacheStats.readerReactivationIdleMSMax.Load(),
	}
}

func (c *Cache) registerMetrics() {
	if c == nil || !c.metrics.registered.CompareAndSwap(false, true) {
		return
	}

	globalCacheStats.activeCaches.Add(1)
	globalCacheStats.configuredCapacityBytes.Add(c.capacity)
	globalCacheStats.piecesCount.Add(int64(c.pieceCount))
	globalCacheStats.residentPieces.Add(c.metrics.residentPieces.Load())
}

func (c *Cache) unregisterMetrics() {
	if c == nil || !c.metrics.registered.CompareAndSwap(true, false) {
		return
	}

	filled := c.filled.Swap(0)
	chunks := c.metrics.inMemoryChunks.Swap(0)
	readers := c.readers.active.Load()
	residentPieces := c.metrics.residentPieces.Swap(0)
	trackedPriorityPieces := c.metrics.priorityTrackedPieces.Swap(0)

	globalCacheStats.activeCaches.Add(-1)
	globalCacheStats.activeReaders.Add(-int64(readers))
	globalCacheStats.configuredCapacityBytes.Add(-c.capacity)
	globalCacheStats.piecesCount.Add(-int64(c.pieceCount))
	globalCacheStats.residentPieces.Add(-residentPieces)
	globalCacheStats.logicalFilledBytes.Add(-filled)
	globalCacheStats.inMemoryChunks.Add(-chunks)
	globalCacheStats.priorityTrackedPieces.Add(-trackedPriorityPieces)
	trimReusableMemPieceChunks(0)
}

func (c *Cache) recordHit() {
	if c == nil {
		return
	}

	c.metrics.hits.Add(1)
	globalCacheStats.hits.Add(1)
}

func (c *Cache) recordMiss() {
	if c == nil {
		return
	}

	c.metrics.misses.Add(1)
	globalCacheStats.misses.Add(1)
}

func (c *Cache) addConfiguredCapacity(delta int64) {
	if c == nil || delta == 0 || !c.metrics.registered.Load() {
		return
	}

	globalCacheStats.configuredCapacityBytes.Add(delta)
}

func (c *Cache) addFilled(delta int64) {
	if c == nil || delta == 0 {
		return
	}

	for {
		old := c.filled.Load()
		next := old + delta

		if next < 0 {
			next = 0
		}

		if c.filled.CompareAndSwap(old, next) {
			if c.metrics.registered.Load() {
				globalCacheStats.logicalFilledBytes.Add(next - old)
			}

			return
		}
	}
}

func (c *Cache) addInMemoryChunks(delta int64) {
	if c == nil || delta == 0 {
		return
	}

	for {
		old := c.metrics.inMemoryChunks.Load()
		next := old + delta

		if next < 0 {
			next = 0
		}

		if c.metrics.inMemoryChunks.CompareAndSwap(old, next) {
			if c.metrics.registered.Load() {
				globalCacheStats.inMemoryChunks.Add(next - old)
			}

			return
		}
	}
}

func (c *Cache) addResidentPieces(delta int64) {
	if c == nil || delta == 0 {
		return
	}

	for {
		old := c.metrics.residentPieces.Load()
		next := old + delta

		if next < 0 {
			next = 0
		}

		if c.metrics.residentPieces.CompareAndSwap(old, next) {
			if c.metrics.registered.Load() {
				globalCacheStats.residentPieces.Add(next - old)
			}

			return
		}
	}
}

func (c *Cache) addActiveReaders(delta int64) {
	if c == nil || delta == 0 {
		return
	}

	c.readers.active.Add(int32(delta))

	if c.metrics.registered.Load() {
		globalCacheStats.activeReaders.Add(delta)
	}
}

func (c *Cache) recordCleanupRun() {
	if c == nil {
		return
	}

	c.metrics.cleanupRuns.Add(1)
	globalCacheStats.cleanupRuns.Add(1)
}

func (c *Cache) recordPrioritySelection(desiredPieces int, budgetLimited bool) {
	if c == nil {
		return
	}

	c.metrics.priorityUpdates.Add(1)
	globalCacheStats.priorityUpdates.Add(1)

	now := time.Now().UnixMilli()
	c.metrics.priorityLastUpdateMS.Store(now)
	globalCacheStats.priorityLastUpdateMS.Store(now)

	if desiredPieces > 0 {
		c.metrics.priorityDesiredPieces.Add(uint64(desiredPieces))
		globalCacheStats.priorityDesiredPieces.Add(uint64(desiredPieces))
	}

	if budgetLimited {
		c.metrics.priorityBudgetLimited.Add(1)
		globalCacheStats.priorityBudgetLimited.Add(1)
	}
}

func (c *Cache) recordPriorityChurn(clearedPieces, setPieces, noopPieces, trackedPieces int) {
	if c == nil {
		return
	}

	if clearedPieces > 0 {
		c.metrics.priorityClearedPieces.Add(uint64(clearedPieces))
		globalCacheStats.priorityClearedPieces.Add(uint64(clearedPieces))
	}

	if setPieces > 0 {
		c.metrics.prioritySetPieces.Add(uint64(setPieces))
		globalCacheStats.prioritySetPieces.Add(uint64(setPieces))
	}

	if noopPieces > 0 {
		c.metrics.priorityNoopPieces.Add(uint64(noopPieces))
		globalCacheStats.priorityNoopPieces.Add(uint64(noopPieces))
	}

	c.setTrackedPriorityPieces(trackedPieces)
}

func (c *Cache) recordRetentionWindowAdjustment(expanded, clamped bool) {
	if c == nil || !c.debugMetricsEnabled() {
		return
	}

	if expanded {
		c.metrics.retentionExpanded.Add(1)
		globalCacheStats.retentionExpanded.Add(1)
	}

	if clamped {
		c.metrics.retentionClamped.Add(1)
		globalCacheStats.retentionClamped.Add(1)
	}
}

func (c *Cache) recordReaderIdleDemotion(idle time.Duration) {
	if c == nil || !c.debugMetricsEnabled() {
		return
	}

	idleMS := durationMilliseconds(idle)
	globalCacheStats.readerIdleDemotions.Add(1)
	globalCacheStats.readerDemotionIdleMSTotal.Add(uint64(idleMS))
	updateAtomicMax(&globalCacheStats.readerDemotionIdleMSMax, idleMS)
}

func (c *Cache) recordReaderReactivation(idle time.Duration) {
	if c == nil || !c.debugMetricsEnabled() {
		return
	}

	idleMS := durationMilliseconds(idle)
	globalCacheStats.readerReactivations.Add(1)
	globalCacheStats.readerReactivationIdleMSTotal.Add(uint64(idleMS))
	updateAtomicMax(&globalCacheStats.readerReactivationIdleMSMax, idleMS)

	if idle >= readerIdleDemotionThreshold {
		globalCacheStats.readerReactivationsAfterIdle.Add(1)
	}
}

func (c *Cache) setTrackedPriorityPieces(trackedPieces int) {
	if c == nil {
		return
	}

	if trackedPieces < 0 {
		trackedPieces = 0
	}

	next := int64(trackedPieces)
	old := c.metrics.priorityTrackedPieces.Swap(next)

	if c.metrics.registered.Load() {
		globalCacheStats.priorityTrackedPieces.Add(next - old)
	}
}

func (c *Cache) debugMetricsEnabled() bool {
	sets := c.currentSettings()
	if sets == nil {
		return false
	}

	return sets.DebugConfig().EnableDebug
}

func durationMilliseconds(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}

	return duration.Milliseconds()
}

func updateAtomicMax(target *atomic.Int64, value int64) {
	for {
		old := target.Load()
		if value <= old {
			return
		}

		if target.CompareAndSwap(old, value) {
			return
		}
	}
}

func (c *Cache) recordCleanedBytes(bytes int64) {
	if c == nil || bytes <= 0 {
		return
	}

	c.metrics.cleanedBytes.Add(uint64(bytes))
	globalCacheStats.cleanedBytes.Add(uint64(bytes))
}
