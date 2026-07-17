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

	readers := c.copyReaders()
	readersState := buildReaderStates(readers)

	cState.Capacity = c.GetCapacity()
	cState.PiecesLength = c.pieceLength
	cState.PiecesCount = c.pieceCount
	cState.Hash = c.hash.HexString()
	cState.Filled = fill
	cState.Pieces = piecesState
	cState.Readers = readersState

	return cState
}

func buildReaderStates(readers []*Reader) []*state.ReaderState {
	if len(readers) == 0 {
		return nil
	}

	activeReaders := countActiveReaders(readers)
	readersState := make([]*state.ReaderState, 0, len(readers))

	for _, r := range readers {
		if r == nil {
			continue
		}

		rng := r.getPiecesRangeForReaders(activeReaders)
		pc := r.getReaderPiece()
		readersState = append(readersState, &state.ReaderState{
			Start:  rng.Start,
			End:    rng.End,
			Reader: pc,
		})
	}

	return readersState
}

func countActiveReaders(readers []*Reader) int {
	activeReaders := 0

	for _, r := range readers {
		if r != nil && r.isActive() {
			activeReaders++
		}
	}

	return activeReaders
}

func (c *Cache) GetCapacity() int64 {
	if c == nil {
		return 0
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.capacity
}

// requestStrategyCapacity reports the live storage bound used by the torrent
// request scheduler. A nil or closed cache remains capped at zero so it cannot
// accidentally become an unlimited request source.
func (c *Cache) requestStrategyCapacity() (int64, bool) {
	if c == nil || c.isClosed.Load() {
		return 0, true
	}

	capacity := c.GetCapacity()
	if capacity < 0 || c.isClosed.Load() {
		return 0, true
	}

	return capacity, true
}

// Filled returns current cached bytes without constructing a full CacheState snapshot.
func (c *Cache) Filled() int64 {
	if c == nil {
		return 0
	}

	return c.filled.Load()
}

// ResidentPieces returns the current number of pieces retained by this cache.
// The atomic load keeps debug diagnostics off the cache mutex hot path.
func (c *Cache) ResidentPieces() int64 {
	if c == nil {
		return 0
	}

	return c.metrics.residentPieces.Load()
}

// RecordHit records a cache hit for metrics.
func (c *Cache) RecordHit() {
	c.recordHit()
}

// RecordMiss records a cache miss for metrics.
func (c *Cache) RecordMiss() {
	c.recordMiss()
}
