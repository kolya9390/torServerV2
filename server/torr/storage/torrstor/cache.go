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

type cacheResidentState struct {
	items map[int]*Piece
	mu    sync.RWMutex
}

// Lock order invariant: priorities.mu is always outside anacrolix/torrent
// client locks. PieceState and SetPriority acquire those internal locks, so
// storage callbacks called by anacrolix must not call methods that acquire
// priorities.mu while the library may already be holding its client lock.
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

type torrentPriorityAPI interface {
	PieceState(id int) torrent.PieceState
	SetPiecePriority(id int, priority torrent.PiecePriority)
}

type realTorrentPriorityAPI struct {
	torrent *torrent.Torrent
}

func (api realTorrentPriorityAPI) PieceState(id int) torrent.PieceState {
	if api.torrent == nil {
		return torrent.PieceState{}
	}

	return api.torrent.PieceState(id)
}

func (api realTorrentPriorityAPI) SetPiecePriority(id int, priority torrent.PiecePriority) {
	if api.torrent == nil {
		return
	}

	api.torrent.Piece(id).SetPriority(priority)
}

type cacheCleanupState struct {
	mu          sync.Mutex
	cond        *sync.Cond
	running     bool
	pending     bool
	queued      atomic.Bool
	lastRunNano atomic.Int64
}

type cacheMetricsState struct {
	registered            atomic.Bool
	hits                  atomic.Uint64
	misses                atomic.Uint64
	inMemoryChunks        atomic.Int64
	residentPieces        atomic.Int64
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
	retentionExpanded     atomic.Uint64
	retentionClamped      atomic.Uint64
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
	priority   torrentPriorityAPI
	readers    cacheReadersState
	resident   cacheResidentState
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
		resident: cacheResidentState{
			items: make(map[int]*Piece),
		},
		priorities: cachePriorityState{
			pieces: make(map[int]torrent.PiecePriority),
		},
	}
	ret.cleanup.cond = sync.NewCond(&ret.cleanup.mu)

	return ret
}

func (c *Cache) currentSettings() *settings.BTSets {
	if c != nil && c.host != nil {
		return c.host.currentSettings()
	}

	return nil
}
