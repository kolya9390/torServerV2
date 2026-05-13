package torrstor

import "server/torr/storage/state"

func (c *Cache) GetState() *state.CacheState {
	cState := new(state.CacheState)
	piecesState := make(map[int]state.ItemState, 0)

	var fill int64

	c.mu.RLock()
	if len(c.pieces) > 0 {
		for _, p := range c.pieces {
			if p.Size.Load() > 0 {
				fill += p.Size.Load()
				piecesState[p.ID] = state.ItemState{
					ID:        p.ID,
					Size:      p.Size.Load(),
					Length:    c.pieceLength,
					Completed: p.Complete,
					Priority:  int(c.torrent.PieceState(p.ID).Priority),
				}
			}
		}
	}
	c.mu.RUnlock()

	readersState := make([]*state.ReaderState, 0)
	if c.Readers() > 0 {
		c.readers.mu.Lock()
		activeReaders := 0

		for r := range c.readers.items {
			if r.isActive() {
				activeReaders++
			}
		}

		for r := range c.readers.items {
			rng := r.getPiecesRangeForReaders(activeReaders)
			pc := r.getReaderPiece()
			readersState = append(readersState, &state.ReaderState{
				Start:  rng.Start,
				End:    rng.End,
				Reader: pc,
			})
		}
		c.readers.mu.Unlock()
	}

	cState.Capacity = c.GetCapacity()
	cState.PiecesLength = c.pieceLength
	cState.PiecesCount = c.pieceCount
	cState.Hash = c.hash.HexString()
	cState.Filled = fill
	cState.Pieces = piecesState
	cState.Readers = readersState

	return cState
}

func (c *Cache) GetCapacity() int64 {
	if c == nil {
		return 0
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.capacity
}

// Filled returns current cached bytes without constructing a full CacheState snapshot.
func (c *Cache) Filled() int64 {
	if c == nil {
		return 0
	}

	return c.filled.Load()
}

// addFilled adjusts total cache occupancy in bytes.
func (c *Cache) addFilled(delta int64) {
	if c == nil || delta == 0 {
		return
	}

	if c.filled.Add(delta) < 0 {
		c.filled.Store(0)
	}
}

// RecordHit records a cache hit for metrics.
func (c *Cache) RecordHit() {
	c.metrics.hits.Add(1)
}

// RecordMiss records a cache miss for metrics.
func (c *Cache) RecordMiss() {
	c.metrics.misses.Add(1)
}

// markUsedLRU moves a piece to the back of the LRU list (most recently used).
// Must be called with c.lru.mu held.
func (c *Cache) markUsedLRU(p *Piece) {
	if p.lruEl == nil {
		p.lruEl = c.lru.list.PushBack(p)
	} else {
		c.lru.list.MoveToBack(p.lruEl)
	}
}

// evictLRU returns the least recently used piece (front of list), or nil if empty.
// Must be called with c.lru.mu held.
func (c *Cache) evictLRU() *Piece {
	if c.lru.list.Len() == 0 {
		return nil
	}

	el := c.lru.list.Front()
	if el == nil {
		return nil
	}

	p, ok := el.Value.(*Piece)
	if !ok {
		return nil
	}

	c.lru.list.Remove(el)
	p.lruEl = nil

	return p
}
