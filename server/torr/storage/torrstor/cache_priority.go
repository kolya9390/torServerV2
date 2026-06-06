package torrstor

import (
	"time"

	"github.com/anacrolix/torrent"
)

type activeReaderSnapshot struct {
	readerPos    int
	readerRAHPos int
	piecesRange  Range
}

func (c *Cache) getActiveReaderRanges() []Range {
	readers := c.snapshotActiveReaders(true)
	if len(readers) == 0 {
		return nil
	}

	ranges := make([]Range, 0, len(readers))
	for _, reader := range readers {
		ranges = append(ranges, reader.piecesRange)
	}

	return mergeRange(ranges)
}

func (c *Cache) copyReaders() []*Reader {
	if c == nil {
		return nil
	}

	c.readers.mu.Lock()
	defer c.readers.mu.Unlock()

	if len(c.readers.items) == 0 {
		return nil
	}

	readers := make([]*Reader, 0, len(c.readers.items))
	for r := range c.readers.items {
		readers = append(readers, r)
	}

	return readers
}

func (c *Cache) snapshotActiveReaders(refreshActivity bool) []activeReaderSnapshot {
	readers := c.copyReaders()
	if len(readers) == 0 {
		return nil
	}

	totalReaders := len(readers)

	for _, reader := range readers {
		if reader != nil && refreshActivity {
			reader.checkReader(totalReaders)
		}
	}

	activeReaders := 0

	for _, reader := range readers {
		if reader != nil && reader.isActive() {
			activeReaders++
		}
	}

	if activeReaders == 0 {
		return nil
	}

	snapshots := make([]activeReaderSnapshot, 0, activeReaders)

	for _, reader := range readers {
		if reader == nil || !reader.isActive() {
			continue
		}

		snapshots = append(snapshots, activeReaderSnapshot{
			readerPos:    reader.getReaderPiece(),
			readerRAHPos: reader.getReaderRAHPiece(),
			piecesRange:  reader.getPiecesRangeForReaders(activeReaders),
		})
	}

	return snapshots
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

	clearedPieces := c.clearPrioritiesOutsideRanges(ranges)
	setPieces, noopPieces, trackedPieces := c.applyDesiredPriorities(c.desiredPriorities(ranges))

	c.recordPriorityChurn(clearedPieces, setPieces, noopPieces, trackedPieces)
}

func (c *Cache) RequestPriorityUpdate() {
	c.queuePriorityUpdate()
}

func (c *Cache) clearPrioritiesOutsideRanges(ranges []Range) int {
	if c == nil || c.isClosed.Load() || c.torrent == nil {
		return 0
	}

	c.priorities.mu.Lock()
	defer c.priorities.mu.Unlock()

	clearedPieces := 0

	for id := range c.priorities.pieces {
		if len(ranges) > 0 && inRanges(ranges, id) {
			continue
		}

		if c.torrent.PieceState(id).Priority != torrent.PiecePriorityNone {
			c.torrent.Piece(id).SetPriority(torrent.PiecePriorityNone)

			clearedPieces++
		}

		delete(c.priorities.pieces, id)
	}

	return clearedPieces
}

func (c *Cache) untrackPriorityPiece(id int) bool {
	if c == nil {
		return false
	}

	c.priorities.mu.Lock()
	defer c.priorities.mu.Unlock()

	if c.priorities.pieces == nil {
		return false
	}

	if _, ok := c.priorities.pieces[id]; !ok {
		return false
	}

	delete(c.priorities.pieces, id)
	c.setTrackedPriorityPieces(len(c.priorities.pieces))

	return true
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

	if connectionsLimit <= 0 {
		connectionsLimit = 1
	}

	if activeReaders <= 0 {
		activeReaders = 1
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
	readers := c.snapshotActiveReaders(false)
	if len(readers) == 0 {
		return nil
	}

	activeReaders := len(readers)
	count := priorityPieceBudget(c.currentNetworkConfig().ConnectionsLimit, activeReaders, c.pieceLength)
	desired := make(map[int]torrent.PiecePriority, activeReaders*count)

	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, reader := range readers {
		if c.isIDInFileBE(ranges, reader.readerPos) {
			continue
		}

		limit := 0

		for i := reader.readerPos; i < reader.piecesRange.End && limit < count; i++ {
			piece, ok := c.pieces[i]
			if !ok {
				continue
			}

			if !piece.Complete {
				desired[i] = maxPiecePriority(
					desired[i],
					desiredPiecePriority(i, reader.readerPos, reader.readerRAHPos),
				)
				limit++
			}
		}
	}

	c.recordPrioritySelection(len(desired), false)

	return desired
}

func (c *Cache) applyDesiredPriorities(desired map[int]torrent.PiecePriority) (int, int, int) {
	if c == nil || c.isClosed.Load() || c.torrent == nil {
		return 0, 0, 0
	}

	c.priorities.mu.Lock()
	defer c.priorities.mu.Unlock()

	if c.priorities.pieces == nil {
		c.priorities.pieces = make(map[int]torrent.PiecePriority)
	}

	setPieces := 0
	noopPieces := 0

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
			noopPieces++

			delete(desired, id)

			continue
		}

		if actual != want {
			c.torrent.Piece(id).SetPriority(want)

			setPieces++
		}

		c.priorities.pieces[id] = want

		delete(desired, id)
	}

	for id, want := range desired {
		if c.torrent.PieceState(id).Priority != want {
			c.torrent.Piece(id).SetPriority(want)

			setPieces++
		}

		c.priorities.pieces[id] = want
	}

	return setPieces, noopPieces, len(c.priorities.pieces)
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

	c.priorities.clearMu.Lock()
	defer c.priorities.clearMu.Unlock()

	if c.priorities.clearTimer != nil {
		c.priorities.clearTimer.Stop()
	}

	c.priorities.clearTimer = time.AfterFunc(clearPriorityDelay, c.runClearPriority)
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

func (c *Cache) clearPriority() {
	if c == nil || c.isClosed.Load() || c.torrent == nil {
		return
	}

	c.clearPrioritiesOutsideRanges(c.getActiveReaderRanges())
}
