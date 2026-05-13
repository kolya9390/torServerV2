package torrstor

import (
	"os"
	"path/filepath"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"

	"server/log"
	"server/torr/utils"
)

func (c *Cache) Init(info *metainfo.Info, hash metainfo.Hash) {
	log.TLogln("Create cache for:", info.Name, hash.HexString())

	cacheCfg := c.currentCacheConfig()

	if c.capacity == 0 {
		c.capacity = info.PieceLength * 4
	}

	c.pieceLength = info.PieceLength
	c.pieceCount = info.NumPieces()
	c.hash = hash

	if cacheCfg.UseDisk {
		name := filepath.Join(cacheCfg.SavePath, hash.HexString())

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
		c.metrics.hits.Add(1)

		return val
	}

	c.metrics.misses.Add(1)

	return &PieceFake{}
}

func (c *Cache) Close() error {
	if c.isClosed.Swap(true) {
		return nil
	}

	if c.torrent != nil {
		log.TLogln("Close cache for:", c.torrent.Name(), c.hash)
	} else {
		log.TLogln("Close cache for:", c.hash)
	}

	c.priorities.clearMu.Lock()
	if c.priorities.clearTimer != nil {
		c.priorities.clearTimer.Stop()
		c.priorities.clearTimer = nil
	}
	c.priorities.clearMu.Unlock()

	if c.host != nil {
		c.host.unregisterCache(c.hash)
	}

	cacheCfg := c.currentCacheConfig()
	if cacheCfg.RemoveOnDrop {
		name := filepath.Join(cacheCfg.SavePath, c.hash.HexString())
		if name != "" && name != "/" {
			for _, v := range c.pieces {
				if v.dPiece != nil {
					_ = os.Remove(v.dPiece.name)
				}
			}

			_ = os.Remove(name)
		}
	}

	c.readers.mu.Lock()
	c.readers.items = nil
	c.readers.mu.Unlock()

	c.priorities.mu.Lock()
	c.priorities.pieces = nil
	c.priorities.mu.Unlock()

	c.mu.Lock()
	c.pieces = nil
	c.mu.Unlock()

	utils.FreeOSMemGC()

	return nil
}

func (c *Cache) removePiece(piece *Piece) {
	if piece == nil {
		return
	}

	if !c.isClosed.Load() {
		piece.Release()
	}
}

func (c *Cache) AdjustRA(readahead int64) {
	if c.currentCacheConfig().SizeBytes == 0 {
		c.mu.Lock()
		c.capacity = readahead * 3
		c.mu.Unlock()
	}

	if c.Readers() > 0 {
		c.readers.mu.Lock()

		readers := make([]*Reader, 0, len(c.readers.items))
		for r := range c.readers.items {
			readers = append(readers, r)
		}
		c.readers.mu.Unlock()

		for _, r := range readers {
			if r == nil || r.Readahead() == readahead {
				continue
			}

			r.SetReadahead(readahead)
		}
	}
}

func (c *Cache) SetCapacity(capacity int64) {
	if c == nil || c.isClosed.Load() || capacity <= 0 {
		return
	}

	minCap := int64(8 << 20)
	if c.pieceLength > 0 && minCap < c.pieceLength {
		minCap = c.pieceLength
	}

	if capacity < minCap {
		capacity = minCap
	}

	c.mu.Lock()

	old := c.capacity
	if old == capacity {
		c.mu.Unlock()

		return
	}

	if c.pieceLength > 0 && absInt64(old-capacity) < c.pieceLength {
		c.mu.Unlock()

		return
	}

	c.capacity = capacity
	c.mu.Unlock()

	if capacity < old {
		c.queueCleanPieces()
	}
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}

	return v
}
