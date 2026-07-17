package torrstor

import (
	"sort"
	"time"
)

// CleanPieces frees cached pieces to make room for new data.
// Called when a new piece buffer is allocated.
func (c *Cache) CleanPieces() {
	if c == nil || c.isClosed.Load() || !c.needsCleanup() {
		return
	}

	c.cleanup.mu.Lock()
	if c.cleanup.running {
		c.cleanup.pending = true
		for c.cleanup.running && !c.isClosed.Load() {
			c.cleanup.cond.Wait()
		}
		c.cleanup.mu.Unlock()

		return
	}

	c.cleanup.running = true
	c.cleanup.pending = false
	c.cleanup.mu.Unlock()

	for {
		madeProgress := c.cleanPiecesPass()

		c.cleanup.mu.Lock()

		shouldContinue := madeProgress && c.cleanup.pending && c.needsCleanup()
		if shouldContinue {
			c.cleanup.pending = false
			c.cleanup.mu.Unlock()

			continue
		}

		c.cleanup.running = false
		c.cleanup.pending = false
		c.cleanup.cond.Broadcast()
		c.cleanup.mu.Unlock()

		return
	}
}

func (c *Cache) cleanPiecesPass() bool {
	curCapacity := c.GetCapacity()

	filledNow := c.filled.Load()
	if filledNow <= curCapacity {
		return false
	}

	c.recordCleanupRun()
	remPieces := c.getRemPieces()
	beforeFilled := filledNow

	if filledNow > curCapacity {
		rems := (filledNow-curCapacity)/c.pieceLength + 1

		sort.Slice(remPieces, func(i, j int) bool {
			return remPieces[i].Accessed < remPieces[j].Accessed
		})

		for _, p := range remPieces {
			c.removePiece(p)

			rems--
			if rems <= 0 {
				cleanedBytes := beforeFilled - c.filled.Load()
				c.recordCleanedBytes(cleanedBytes)

				return cleanedBytes > 0
			}
		}
	}

	cleanedBytes := beforeFilled - c.filled.Load()
	c.recordCleanedBytes(cleanedBytes)

	return cleanedBytes > 0
}

func (c *Cache) needsCleanup() bool {
	return c != nil && !c.isClosed.Load() && c.filled.Load() > c.GetCapacity()
}

func (c *Cache) getRemPieces() []*Piece {
	ranges := c.getActiveReaderRanges()
	residentPieces := c.copyResidentPieces()

	return c.removableResidentPieces(residentPieces, ranges)
}

func (c *Cache) removableResidentPieces(residentPieces []*Piece, ranges []Range) []*Piece {
	piecesRemove := make([]*Piece, 0, 64)

	for _, p := range residentPieces {
		if p == nil {
			continue
		}

		pSize := p.Size.Load()
		if pSize == 0 {
			continue
		}

		id := p.ID
		if !inRanges(ranges, id) && !c.isIDInFileBEFast(ranges, id) {
			piecesRemove = append(piecesRemove, p)
		}
	}

	return piecesRemove
}

func (c *Cache) copyResidentPieces() []*Piece {
	if c == nil {
		return nil
	}

	c.resident.mu.RLock()
	defer c.resident.mu.RUnlock()

	if len(c.resident.items) == 0 {
		return nil
	}

	pieces := make([]*Piece, 0, len(c.resident.items))
	for _, piece := range c.resident.items {
		pieces = append(pieces, piece)
	}

	return pieces
}

func (c *Cache) markResidentPiece(piece *Piece) {
	if c == nil || piece == nil {
		return
	}

	c.resident.mu.Lock()
	inserted := false

	if c.resident.items != nil {
		_, exists := c.resident.items[piece.ID]
		if !exists {
			c.resident.items[piece.ID] = piece
			inserted = true
		}
	}
	c.resident.mu.Unlock()

	if inserted {
		c.addResidentPieces(1)
	}
}

func (c *Cache) unmarkResidentPiece(piece *Piece) {
	if c == nil || piece == nil {
		return
	}

	c.resident.mu.Lock()

	exists := false
	if c.resident.items != nil {
		_, exists = c.resident.items[piece.ID]
		delete(c.resident.items, piece.ID)
	}
	c.resident.mu.Unlock()

	if exists {
		c.addResidentPieces(-1)
	}
}

func (c *Cache) queueCleanPieces() {
	if c == nil || c.isClosed.Load() {
		return
	}

	now := time.Now().UnixNano()
	last := c.cleanup.lastRunNano.Load()

	if last != 0 && time.Duration(now-last) < minCleanInterval {
		return
	}

	if !c.cleanup.lastRunNano.CompareAndSwap(last, now) {
		return
	}

	if !c.cleanup.queued.CompareAndSwap(false, true) {
		return
	}

	go func() {
		defer c.cleanup.queued.Store(false)

		c.CleanPieces()
	}()
}

// isIDInFileBEFast is a non-locking variant for use inside locked sections.
func (c *Cache) isIDInFileBEFast(ranges []Range, id int) bool {
	fileRangeNotDelete := max(c.pieceLength, 8<<20)

	for _, rng := range ranges {
		if rng.File == nil {
			continue
		}

		ss := int(rng.File.Offset() / c.pieceLength)
		se := int((rng.File.Offset() + fileRangeNotDelete) / c.pieceLength)
		es := int((rng.File.Offset() + rng.File.Length() - fileRangeNotDelete) / c.pieceLength)
		ee := int((rng.File.Offset() + rng.File.Length()) / c.pieceLength)

		if id >= ss && id < se || id > es && id <= ee {
			return true
		}
	}

	return false
}
