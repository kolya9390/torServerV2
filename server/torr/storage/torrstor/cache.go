package torrstor

import (
	"container/list"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anacrolix/torrent"

	"server/log"
	"server/settings"
	"server/torr/storage/state"
	"server/torr/utils"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

// CacheMetricsRecorder is an optional callback for recording cache metrics.
// Set by the metrics package during initialization.
var CacheMetricsRecorder func(hits, misses uint64)

type Cache struct {
	storage.TorrentImpl
	storage *Storage

	capacity int64
	filled   int64
	hash     metainfo.Hash

	pieceLength int64
	pieceCount  int

	pieces map[int]*Piece
	mu     sync.RWMutex // protects pieces map and filled counter

	readers   map[*Reader]struct{}
	muReaders sync.Mutex

	isRemove atomic.Bool
	isClosed atomic.Bool
	muRemove sync.Mutex
	torrent  *torrent.Torrent

	// Cache metrics (atomic counters)
	hits   atomic.Uint64
	misses atomic.Uint64

	// LRU list for O(1) eviction order tracking.
	// Front = least recently used, Back = most recently used.
	lru   *list.List
	lruMu sync.Mutex // protects lru list operations
}

func NewCache(capacity int64, storage *Storage) *Cache {
	ret := &Cache{
		capacity: capacity,
		filled:   0,
		pieces:   make(map[int]*Piece),
		storage:  storage,
		readers:  make(map[*Reader]struct{}),
		lru:      list.New(),
	}

	return ret
}

func (c *Cache) Init(info *metainfo.Info, hash metainfo.Hash) {
	log.TLogln("Create cache for:", info.Name, hash.HexString())

	if c.capacity == 0 {
		c.capacity = info.PieceLength * 4
	}

	c.pieceLength = info.PieceLength
	c.pieceCount = info.NumPieces()
	c.hash = hash

	if settings.BTsets.UseDisk {
		name := filepath.Join(settings.BTsets.TorrentsSavePath, hash.HexString())

		err := os.MkdirAll(name, 0o777)
		if err != nil {
			log.TLogln("Error create dir:", err)
		}
	}

	for i := range c.pieceCount {
		c.pieces[i] = NewPiece(i, c)
	}
}

func (c *Cache) SetTorrent(torr *torrent.Torrent) {
	c.torrent = torr
}

func (c *Cache) Piece(m metainfo.Piece) storage.PieceImpl {
	c.mu.RLock()
	val, ok := c.pieces[m.Index()]
	c.mu.RUnlock()

	if ok {
		c.hits.Add(1)

		return val
	}

	c.misses.Add(1)

	return &PieceFake{}
}

func (c *Cache) Close() error {
	if c.torrent != nil {
		log.TLogln("Close cache for:", c.torrent.Name(), c.hash)
	} else {
		log.TLogln("Close cache for:", c.hash)
	}

	c.isClosed.Store(true)

	delete(c.storage.caches, c.hash)

	if settings.BTsets.RemoveCacheOnDrop {
		name := filepath.Join(settings.BTsets.TorrentsSavePath, c.hash.HexString())
		if name != "" && name != "/" {
			for _, v := range c.pieces {
				if v.dPiece != nil {
					_ = os.Remove(v.dPiece.name)
				}
			}

			_ = os.Remove(name)
		}
	}

	c.muReaders.Lock()
	c.readers = nil
	c.muReaders.Unlock()

	c.mu.Lock()
	c.pieces = nil
	c.mu.Unlock()

	utils.FreeOSMemGC()

	return nil
}

func (c *Cache) removePiece(piece *Piece) {
	if !c.isClosed.Load() {
		piece.Release()
	}
}

func (c *Cache) AdjustRA(readahead int64) {
	if settings.BTsets.CacheSize == 0 {
		c.mu.Lock()
		c.capacity = readahead * 3
		c.mu.Unlock()
	}

	if c.Readers() > 0 {
		c.muReaders.Lock()
		for r := range c.readers {
			r.SetReadahead(readahead)
		}
		c.muReaders.Unlock()
	}
}

func (c *Cache) GetState() *state.CacheState {
	cState := new(state.CacheState)

	piecesState := make(map[int]state.ItemState, 0)

	var fill int64 = 0

	c.mu.RLock()
	if len(c.pieces) > 0 {
		for _, p := range c.pieces {
			if p.Size.Load() > 0 {
				fill += p.Size.Load()
				piecesState[p.Id] = state.ItemState{
					Id:        p.Id,
					Size:      p.Size.Load(),
					Length:    c.pieceLength,
					Completed: p.Complete,
					Priority:  int(c.torrent.PieceState(p.Id).Priority),
				}
			}
		}
	}
	c.mu.RUnlock()

	readersState := make([]*state.ReaderState, 0)

	if c.Readers() > 0 {
		c.muReaders.Lock()
		for r := range c.readers {
			rng := r.getPiecesRange()
			pc := r.getReaderPiece()
			readersState = append(readersState, &state.ReaderState{
				Start:  rng.Start,
				End:    rng.End,
				Reader: pc,
			})
		}
		c.muReaders.Unlock()
	}

	c.filled = fill
	cState.Capacity = c.capacity
	cState.PiecesLength = c.pieceLength
	cState.PiecesCount = c.pieceCount
	cState.Hash = c.hash.HexString()
	cState.Filled = fill
	cState.Pieces = piecesState
	cState.Readers = readersState

	return cState
}

func (c *Cache) cleanPieces() {
	if c.isRemove.Load() || c.isClosed.Load() {
		return
	}

	c.muRemove.Lock()
	if c.isRemove.Load() {
		c.muRemove.Unlock()

		return
	}

	c.isRemove.Store(true)

	defer func() { c.isRemove.Store(false) }()
	c.muRemove.Unlock()

	remPieces := c.getRemPieces()
	c.mu.RLock()
	curFilled := c.filled
	curCapacity := c.capacity
	c.mu.RUnlock()

	if curFilled > curCapacity {
		rems := (curFilled-curCapacity)/c.pieceLength + 1

		for rems > 0 {
			// Use LRU eviction when available
			c.lruMu.Lock()
			p := c.evictLRU()
			c.lruMu.Unlock()

			if p == nil {
				break
			}

			c.removePiece(p)

			rems--
		}

		// Fallback: getRemPieces for non-LRU pieces
		for _, p := range remPieces {
			if rems <= 0 {
				break
			}

			c.removePiece(p)

			rems--
		}

		utils.FreeOSMemGC()
	}
}

func (c *Cache) getRemPieces() []*Piece {
	piecesRemove := make([]*Piece, 0, 64)
	fill := int64(0)

	ranges := make([]Range, 0)

	c.muReaders.Lock()
	for r := range c.readers {
		r.checkReader()

		if r.isUse {
			ranges = append(ranges, r.getPiecesRange())
		}
	}
	c.muReaders.Unlock()

	ranges = mergeRange(ranges)

	// Build a boolean lookup for O(1) range checks
	inRangeSet := make(map[int]bool, len(ranges)*10)

	for _, rng := range ranges {
		for id := rng.Start; id <= rng.End; id++ {
			inRangeSet[id] = true
		}
	}

	c.mu.RLock()

	pieces := c.pieces
	for id, p := range pieces {
		if p.Size.Load() > 0 {
			fill += p.Size.Load()
		}

		if !inRangeSet[id] && p.Size.Load() > 0 && !c.isIdInFileBEFast(ranges, id) {
			piecesRemove = append(piecesRemove, p)
		}
	}
	c.mu.RUnlock()

	c.clearPriority()
	c.setLoadPriority(ranges)

	// Partial sort: only need the oldest N pieces, not full sort
	if len(piecesRemove) > 128 {
		// For large eviction sets, use partial sort
		sort.Slice(piecesRemove, func(i, j int) bool {
			return piecesRemove[i].Accessed < piecesRemove[j].Accessed
		})
	}

	c.mu.Lock()
	c.filled = fill
	c.mu.Unlock()

	// Report metrics via callback if registered
	if CacheMetricsRecorder != nil {
		CacheMetricsRecorder(c.hits.Load(), c.misses.Load())
	}

	return piecesRemove
}

func (c *Cache) setLoadPriority(ranges []Range) {
	c.muReaders.Lock()

	readerCount := len(c.readers)
	if readerCount == 0 {
		c.muReaders.Unlock()

		return
	}

	count := settings.BTsets.ConnectionsLimit / readerCount // max concurrent loading blocks

	for r := range c.readers {
		if !r.isUse {
			continue
		}

		if c.isIdInFileBE(ranges, r.getReaderPiece()) {
			continue
		}

		readerPos := r.getReaderPiece()
		readerRAHPos := r.getReaderRAHPiece()
		end := r.getPiecesRange().End
		limit := 0

		for i := readerPos; i < end && limit < count; i++ {
			if !c.pieces[i].Complete {
				if i == readerPos {
					c.torrent.Piece(i).SetPriority(torrent.PiecePriorityNow)
				} else if i == readerPos+1 {
					c.torrent.Piece(i).SetPriority(torrent.PiecePriorityNext)
				} else if i > readerPos && i <= readerRAHPos {
					c.torrent.Piece(i).SetPriority(torrent.PiecePriorityReadahead)
				} else if i > readerRAHPos && i <= readerRAHPos+5 && c.torrent.PieceState(i).Priority != torrent.PiecePriorityHigh {
					c.torrent.Piece(i).SetPriority(torrent.PiecePriorityHigh)
				} else if i > readerRAHPos+5 && c.torrent.PieceState(i).Priority != torrent.PiecePriorityNormal {
					c.torrent.Piece(i).SetPriority(torrent.PiecePriorityNormal)
				}

				limit++
			}
		}
	}
	c.muReaders.Unlock()
}

func (c *Cache) isIdInFileBE(ranges []Range, id int) bool {
	// keep 8/16 MB
	FileRangeNotDelete := max(c.pieceLength, 8<<20)

	for _, rng := range ranges {
		ss := int(rng.File.Offset() / c.pieceLength)
		se := int((rng.File.Offset() + FileRangeNotDelete) / c.pieceLength)

		es := int((rng.File.Offset() + rng.File.Length() - FileRangeNotDelete) / c.pieceLength)
		ee := int((rng.File.Offset() + rng.File.Length()) / c.pieceLength)

		if id >= ss && id < se || id > es && id <= ee {
			return true
		}
	}

	return false
}

// isIdInFileBEFast is a non-locking variant for use inside locked sections.
func (c *Cache) isIdInFileBEFast(ranges []Range, id int) bool {
	FileRangeNotDelete := max(c.pieceLength, 8<<20)

	for _, rng := range ranges {
		ss := int(rng.File.Offset() / c.pieceLength)
		se := int((rng.File.Offset() + FileRangeNotDelete) / c.pieceLength)
		es := int((rng.File.Offset() + rng.File.Length() - FileRangeNotDelete) / c.pieceLength)
		ee := int((rng.File.Offset() + rng.File.Length()) / c.pieceLength)

		if id >= ss && id < se || id > es && id <= ee {
			return true
		}
	}

	return false
}

//////////////////
// Reader section
////////

func (c *Cache) NewReader(file *torrent.File) *Reader {
	return newReader(file, c)
}

func (c *Cache) GetUseReaders() int {
	if c == nil {
		return 0
	}

	c.muReaders.Lock()
	defer c.muReaders.Unlock()

	readers := 0
	for reader := range c.readers {
		if reader.isUse {
			readers++
		}
	}

	return readers
}

func (c *Cache) Readers() int {
	if c == nil {
		return 0
	}

	c.muReaders.Lock()
	defer c.muReaders.Unlock()

	if c.readers == nil {
		return 0
	}

	return len(c.readers)
}

func (c *Cache) CloseReader(r *Reader) {
	r.cache.muReaders.Lock()
	r.Close()
	delete(r.cache.readers, r)
	r.cache.muReaders.Unlock()

	go c.clearPriority()
}

func (c *Cache) clearPriority() {
	if c == nil || c.isClosed.Load() || c.torrent == nil {
		return
	}

	time.Sleep(time.Second)

	ranges := make([]Range, 0)

	c.muReaders.Lock()
	for r := range c.readers {
		r.checkReader()

		if r.isUse {
			ranges = append(ranges, r.getPiecesRange())
		}
	}
	c.muReaders.Unlock()

	ranges = mergeRange(ranges)

	c.mu.RLock()
	pieces := c.pieces
	c.mu.RUnlock()

	for id := range pieces {
		if len(ranges) > 0 {
			if !inRanges(ranges, id) {
				if c.torrent.PieceState(id).Priority != torrent.PiecePriorityNone {
					c.torrent.Piece(id).SetPriority(torrent.PiecePriorityNone)
				}
			}
		} else {
			if c.torrent.PieceState(id).Priority != torrent.PiecePriorityNone {
				c.torrent.Piece(id).SetPriority(torrent.PiecePriorityNone)
			}
		}
	}
}

func (c *Cache) GetCapacity() int64 {
	if c == nil {
		return 0
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.capacity
}

// RecordHit records a cache hit for metrics.
func (c *Cache) RecordHit() {
	c.hits.Add(1)
}

// RecordMiss records a cache miss for metrics.
func (c *Cache) RecordMiss() {
	c.misses.Add(1)
}

// markUsedLRU moves a piece to the back of the LRU list (most recently used).
// Must be called with c.lruMu held.
func (c *Cache) markUsedLRU(p *Piece) {
	if p.lruEl == nil {
		p.lruEl = c.lru.PushBack(p)
	} else {
		c.lru.MoveToBack(p.lruEl)
	}
}

// evictLRU returns the least recently used piece (front of list), or nil if empty.
// Must be called with c.lruMu held.
func (c *Cache) evictLRU() *Piece {
	if c.lru.Len() == 0 {
		return nil
	}

	el := c.lru.Front()
	if el == nil {
		return nil
	}

	p := el.Value.(*Piece)
	c.lru.Remove(el)

	p.lruEl = nil

	return p
}
