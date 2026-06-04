package torrstor

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"

	"server/settings"

	"github.com/anacrolix/torrent/metainfo"
)

const (
	minCleanInterval          = 250 * time.Millisecond
	minPriorityUpdateInterval = time.Second
	clearPriorityDelay        = time.Second
)

type cacheReadersState struct {
	items  map[*Reader]struct{}
	mu     sync.Mutex
	active atomic.Int32
}

type cachePriorityState struct {
	mu            sync.Mutex
	pieces        map[int]torrent.PiecePriority
	updateRunning atomic.Bool
	updateQueued  atomic.Bool
	lastQueueNano atomic.Int64
	clearRunning  atomic.Bool
	clearTimer    *time.Timer
	clearMu       sync.Mutex
}

type cacheCleanupState struct {
	removing    atomic.Bool
	queued      atomic.Bool
	lastRunNano atomic.Int64
	mu          sync.Mutex
}

type cacheMetricsState struct {
	registered            atomic.Bool
	hits                  atomic.Uint64
	misses                atomic.Uint64
	inMemoryChunks        atomic.Int64
	cleanupRuns           atomic.Uint64
	cleanedBytes          atomic.Uint64
	priorityUpdates       atomic.Uint64
	priorityDesiredPieces atomic.Uint64
	priorityBudgetLimited atomic.Uint64
	priorityClearedPieces atomic.Uint64
	prioritySetPieces     atomic.Uint64
	priorityNoopPieces    atomic.Uint64
	priorityTrackedPieces atomic.Int64
	priorityLastUpdateMS  atomic.Int64
}

type cacheHost interface {
	currentSettings() *settings.BTSets
	unregisterCache(hash metainfo.Hash)
}

type Cache struct {
	storage.TorrentImpl
	host cacheHost

	capacity int64
	filled   atomic.Int64
	hash     metainfo.Hash

	pieceLength int64
	pieceCount  int

	pieces map[int]*Piece
	mu     sync.RWMutex // protects pieces map

	isClosed   atomic.Bool
	torrent    *torrent.Torrent
	readers    cacheReadersState
	priorities cachePriorityState
	cleanup    cacheCleanupState
	metrics    cacheMetricsState
}

func NewCache(capacity int64, host cacheHost) *Cache {
	ret := &Cache{
		capacity: capacity,
		pieces:   make(map[int]*Piece),
		host:     host,
		readers: cacheReadersState{
			items: make(map[*Reader]struct{}),
		},
		priorities: cachePriorityState{
			pieces: make(map[int]torrent.PiecePriority),
		},
	}

	return ret
}

func (c *Cache) currentSettings() *settings.BTSets {
	if c != nil && c.host != nil {
		return c.host.currentSettings()
	}

	return nil
}
