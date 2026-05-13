package torrstor

import (
	"sort"
	"time"
)

// CleanPieces frees cached pieces to make room for new data.
// Called when a new piece buffer is allocated.
func (c *Cache) CleanPieces() {
	if c.cleanup.removing.Load() || c.isClosed.Load() {
		return
	}

	c.mu.RLock()
	curCapacity := c.capacity
	c.mu.RUnlock()

	filledNow := c.filled.Load()
	if filledNow <= curCapacity {
		return
	}

	c.cleanup.mu.Lock()
	if c.cleanup.removing.Load() {
		c.cleanup.mu.Unlock()
		return
	}

	c.cleanup.removing.Store(true)
	defer func() { c.cleanup.removing.Store(false) }()
	c.cleanup.mu.Unlock()

	remPieces := c.getRemPieces()

	if c.filled.Load() > curCapacity {
		rems := (c.filled.Load()-curCapacity)/c.pieceLength + 1

		sort.Slice(remPieces, func(i, j int) bool {
			return remPieces[i].Accessed < remPieces[j].Accessed
		})

		for _, p := range remPieces {
			c.removePiece(p)

			rems--
			if rems <= 0 {
				return
			}
		}
	}
}

func (c *Cache) getRemPieces() []*Piece {
	piecesRemove := make([]*Piece, 0, 64)
	ranges := c.getActiveReaderRanges()

	c.mu.RLock()
	pieces := c.pieces
	for id, p := range pieces {
		pSize := p.Size.Load()
		if pSize == 0 {
			continue
		}

		if !inRanges(ranges, id) && !c.isIDInFileBEFast(ranges, id) {
			piecesRemove = append(piecesRemove, p)
		}
	}
	c.mu.RUnlock()

	if CacheMetricsRecorder != nil {
		CacheMetricsRecorder(c.metrics.hits.Load(), c.metrics.misses.Load())
	}

	return piecesRemove
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

func (c *Cache) cleanHeadroom() int64 {
	if c == nil {
		return 8 << 20
	}

	headroom := c.pieceLength * 8
	if headroom < 8<<20 {
		headroom = 8 << 20
	}

	if headroom > 64<<20 {
		headroom = 64 << 20
	}

	c.mu.RLock()
	capacity := c.capacity
	c.mu.RUnlock()

	if capacity > 0 && headroom > capacity/2 {
		headroom = capacity / 2
	}

	if headroom < c.pieceLength {
		headroom = c.pieceLength
	}

	return headroom
}

func (c *Cache) isIDInFileBE(ranges []Range, id int) bool {
	fileRangeNotDelete := max(c.pieceLength, 8<<20)

	for _, rng := range ranges {
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

// isIDInFileBEFast is a non-locking variant for use inside locked sections.
func (c *Cache) isIDInFileBEFast(ranges []Range, id int) bool {
	fileRangeNotDelete := max(c.pieceLength, 8<<20)

	for _, rng := range ranges {
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
