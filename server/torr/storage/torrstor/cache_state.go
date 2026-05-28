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

// RecordHit records a cache hit for metrics.
func (c *Cache) RecordHit() {
	c.recordHit()
}

// RecordMiss records a cache miss for metrics.
func (c *Cache) RecordMiss() {
	c.recordMiss()
}
