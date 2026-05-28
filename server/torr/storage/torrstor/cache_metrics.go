package torrstor

import "sync/atomic"

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
	InMemoryChunks          int64  `json:"in_memory_chunks"`
	ReusableChunks          int64  `json:"reusable_chunks"`
	ReusableBytes           int64  `json:"reusable_bytes"`
	CleanupRuns             uint64 `json:"cleanup_runs"`
	CleanedBytes            uint64 `json:"cleaned_bytes"`
	Hits                    uint64 `json:"hits"`
	Misses                  uint64 `json:"misses"`
}

type cacheStatsCounters struct {
	activeCaches            atomic.Int64
	activeReaders           atomic.Int64
	logicalFilledBytes      atomic.Int64
	configuredCapacityBytes atomic.Int64
	piecesCount             atomic.Int64
	inMemoryChunks          atomic.Int64
	cleanupRuns             atomic.Uint64
	cleanedBytes            atomic.Uint64
	hits                    atomic.Uint64
	misses                  atomic.Uint64
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
		InMemoryChunks:          globalCacheStats.inMemoryChunks.Load(),
		ReusableChunks:          reusableChunks,
		ReusableBytes:           reusableChunks * memPieceChunkSize,
		CleanupRuns:             globalCacheStats.cleanupRuns.Load(),
		CleanedBytes:            globalCacheStats.cleanedBytes.Load(),
		Hits:                    globalCacheStats.hits.Load(),
		Misses:                  globalCacheStats.misses.Load(),
	}
}

func (c *Cache) registerMetrics() {
	if c == nil || !c.metrics.registered.CompareAndSwap(false, true) {
		return
	}

	globalCacheStats.activeCaches.Add(1)
	globalCacheStats.configuredCapacityBytes.Add(c.capacity)
	globalCacheStats.piecesCount.Add(int64(c.pieceCount))
}

func (c *Cache) unregisterMetrics() {
	if c == nil || !c.metrics.registered.CompareAndSwap(true, false) {
		return
	}

	filled := c.filled.Swap(0)
	chunks := c.metrics.inMemoryChunks.Swap(0)
	readers := c.readers.active.Load()

	globalCacheStats.activeCaches.Add(-1)
	globalCacheStats.activeReaders.Add(-int64(readers))
	globalCacheStats.configuredCapacityBytes.Add(-c.capacity)
	globalCacheStats.piecesCount.Add(-int64(c.pieceCount))
	globalCacheStats.logicalFilledBytes.Add(-filled)
	globalCacheStats.inMemoryChunks.Add(-chunks)
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

func (c *Cache) recordCleanedBytes(bytes int64) {
	if c == nil || bytes <= 0 {
		return
	}

	c.metrics.cleanedBytes.Add(uint64(bytes))
	globalCacheStats.cleanedBytes.Add(uint64(bytes))
}
