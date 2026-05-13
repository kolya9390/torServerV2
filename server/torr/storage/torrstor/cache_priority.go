package torrstor

import (
	"time"

	"github.com/anacrolix/torrent"
)

func (c *Cache) getActiveReaderRanges() []Range {
	ranges := make([]Range, 0)

	c.readers.mu.Lock()
	totalReaders := len(c.readers.items)
	activeReaders := 0

	for r := range c.readers.items {
		r.checkReader(totalReaders)

		if r.isActive() {
			activeReaders++
		}
	}

	if activeReaders > 0 {
		for r := range c.readers.items {
			if r.isActive() {
				ranges = append(ranges, r.getPiecesRangeForReaders(activeReaders))
			}
		}
	}
	c.readers.mu.Unlock()

	return mergeRange(ranges)
}

// UpdatePriorities refreshes piece download priorities based on reader positions.
func (c *Cache) UpdatePriorities() {
	if c == nil || c.isClosed.Load() || c.torrent == nil {
		return
	}

	if !c.priorities.updateRunning.CompareAndSwap(false, true) {
		return
	}
	defer c.priorities.updateRunning.Store(false)

	ranges := c.getActiveReaderRanges()
	if ranges == nil {
		return
	}

	c.clearPrioritiesOutsideRanges(ranges)
	c.applyDesiredPriorities(c.desiredPriorities(ranges))
}

func (c *Cache) RequestPriorityUpdate() {
	c.queuePriorityUpdate()
}

func (c *Cache) clearPrioritiesOutsideRanges(ranges []Range) {
	if c == nil || c.isClosed.Load() || c.torrent == nil {
		return
	}

	c.priorities.mu.Lock()
	defer c.priorities.mu.Unlock()

	for id := range c.priorities.pieces {
		if len(ranges) > 0 && inRanges(ranges, id) {
			continue
		}

		if c.torrent.PieceState(id).Priority != torrent.PiecePriorityNone {
			c.torrent.Piece(id).SetPriority(torrent.PiecePriorityNone)
		}

		delete(c.priorities.pieces, id)
	}
}

func (c *Cache) queuePriorityUpdate() {
	if c == nil || c.isClosed.Load() {
		return
	}

	now := time.Now().UnixNano()
	last := c.priorities.lastQueueNano.Load()

	if last != 0 && time.Duration(now-last) < minPriorityUpdateInterval {
		return
	}

	if !c.priorities.lastQueueNano.CompareAndSwap(last, now) {
		return
	}

	if !c.priorities.updateQueued.CompareAndSwap(false, true) {
		return
	}

	go func() {
		defer c.priorities.updateQueued.Store(false)
		c.UpdatePriorities()
	}()
}

func priorityPieceBudget(connectionsLimit, activeReaders int, pieceLength int64) int {
	_ = pieceLength

	if activeReaders <= 0 {
		activeReaders = 1
	}

	if connectionsLimit <= 0 {
		connectionsLimit = 1
	}

	budget := connectionsLimit / activeReaders
	if budget < 1 {
		budget = 1
	}

	return budget
}

func maxPiecePriority(current, next torrent.PiecePriority) torrent.PiecePriority {
	if next > current {
		return next
	}

	return current
}

func desiredPiecePriority(pieceID, readerPos, readerRAHPos int) torrent.PiecePriority {
	switch {
	case pieceID == readerPos:
		return torrent.PiecePriorityNow
	case pieceID == readerPos+1:
		return torrent.PiecePriorityNext
	case pieceID > readerPos && pieceID <= readerRAHPos:
		return torrent.PiecePriorityReadahead
	case pieceID > readerRAHPos && pieceID <= readerRAHPos+5:
		return torrent.PiecePriorityHigh
	default:
		return torrent.PiecePriorityNormal
	}
}

func (c *Cache) desiredPriorities(ranges []Range) map[int]torrent.PiecePriority {
	c.readers.mu.Lock()
	defer c.readers.mu.Unlock()

	readerCount := len(c.readers.items)
	if readerCount == 0 {
		return nil
	}

	activeReaders := 0
	for r := range c.readers.items {
		if r.isActive() {
			activeReaders++
		}
	}

	if activeReaders == 0 {
		return nil
	}

	count := priorityPieceBudget(c.currentNetworkConfig().ConnectionsLimit, activeReaders, c.pieceLength)
	desired := make(map[int]torrent.PiecePriority, activeReaders*count)

	for r := range c.readers.items {
		if !r.isActive() {
			continue
		}

		if c.isIDInFileBE(ranges, r.getReaderPiece()) {
			continue
		}

		readerPos := r.getReaderPiece()
		readerRAHPos := r.getReaderRAHPiece()
		end := r.getPiecesRangeForReaders(activeReaders).End
		limit := 0

		for i := readerPos; i < end && limit < count; i++ {
			piece, ok := c.pieces[i]
			if !ok {
				continue
			}

			if !piece.Complete {
				desired[i] = maxPiecePriority(desired[i], desiredPiecePriority(i, readerPos, readerRAHPos))
				limit++
			}
		}
	}

	return desired
}

func (c *Cache) applyDesiredPriorities(desired map[int]torrent.PiecePriority) {
	if c == nil || c.isClosed.Load() || c.torrent == nil {
		return
	}

	c.priorities.mu.Lock()
	defer c.priorities.mu.Unlock()

	if c.priorities.pieces == nil {
		c.priorities.pieces = make(map[int]torrent.PiecePriority)
	}

	for id, tracked := range c.priorities.pieces {
		want, keep := desired[id]
		actual := c.torrent.PieceState(id).Priority

		if !keep {
			if actual != torrent.PiecePriorityNone {
				c.torrent.Piece(id).SetPriority(torrent.PiecePriorityNone)
			}

			delete(c.priorities.pieces, id)
			continue
		}

		if tracked == want && actual == want {
			delete(desired, id)
			continue
		}

		if actual != want {
			c.torrent.Piece(id).SetPriority(want)
		}

		c.priorities.pieces[id] = want
		delete(desired, id)
	}

	for id, want := range desired {
		if c.torrent.PieceState(id).Priority != want {
			c.torrent.Piece(id).SetPriority(want)
		}

		c.priorities.pieces[id] = want
	}
}

func (c *Cache) NewReader(file *torrent.File) *Reader {
	return newReader(file, c)
}

func (c *Cache) GetUseReaders() int {
	if c == nil {
		return 0
	}

	return int(c.readers.active.Load())
}

func (c *Cache) Readers() int {
	if c == nil {
		return 0
	}

	c.readers.mu.Lock()
	defer c.readers.mu.Unlock()

	if c.readers.items == nil {
		return 0
	}

	return len(c.readers.items)
}

func (c *Cache) CloseReader(r *Reader) {
	if r == nil || r.cache == nil {
		return
	}

	r.cache.readers.mu.Lock()
	delete(r.cache.readers.items, r)
	r.cache.readers.mu.Unlock()

	r.Close()

	c.clearPriorityAsync()
}

func (c *Cache) clearPriorityAsync() {
	if c == nil || c.isClosed.Load() || c.torrent == nil {
		return
	}

	delay := c.clearPriorityDelay()
	c.priorities.clearMu.Lock()
	defer c.priorities.clearMu.Unlock()

	if c.priorities.clearTimer != nil {
		c.priorities.clearTimer.Stop()
	}

	c.priorities.clearTimer = time.AfterFunc(delay, c.runClearPriority)
}

func (c *Cache) runClearPriority() {
	if c == nil || c.isClosed.Load() || c.torrent == nil {
		return
	}

	if !c.priorities.clearRunning.CompareAndSwap(false, true) {
		return
	}
	defer c.priorities.clearRunning.Store(false)

	c.clearPriority()
}

func (c *Cache) clearPriorityDelay() time.Duration {
	return time.Second
}

func (c *Cache) clearPriority() {
	if c == nil || c.isClosed.Load() || c.torrent == nil {
		return
	}

	ranges := make([]Range, 0)

	c.readers.mu.Lock()
	totalReaders := len(c.readers.items)
	activeReaders := 0

	for r := range c.readers.items {
		r.checkReader(totalReaders)
		if r.isActive() {
			activeReaders++
		}
	}

	if activeReaders > 0 {
		for r := range c.readers.items {
			if r.isActive() {
				ranges = append(ranges, r.getPiecesRangeForReaders(activeReaders))
			}
		}
	}
	c.readers.mu.Unlock()

	ranges = mergeRange(ranges)
	c.clearPrioritiesOutsideRanges(ranges)
}
