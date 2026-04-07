package torrstor

import (
	"container/list"
	"sync/atomic"
	"time"

	"server/settings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"
)

// Piece represents a torrent piece in the cache.
type Piece struct {
	storage.PieceImpl `json:"-"`

	ID int `json:"-"`

	// Size is accessed concurrently by WriteAt/ReadAt and cleanPieces.
	Size atomic.Int64 `json:"size"`

	Complete bool  `json:"complete"`
	Accessed int64 `json:"accessed"`

	mPiece *MemPiece
	dPiece *DiskPiece

	cache *Cache

	// LRU element for O(1) removal from cache LRU list
	lruEl *list.Element
}

func NewPiece(id int, cache *Cache) *Piece {
	p := &Piece{
		ID:    id,
		cache: cache,
	}

	if !settings.BTsets.UseDisk {
		p.mPiece = NewMemPiece(p)
	} else {
		p.dPiece = NewDiskPiece(p)
	}

	return p
}

func (p *Piece) WriteAt(b []byte, off int64) (n int, err error) {
	if !settings.BTsets.UseDisk {
		return p.mPiece.WriteAt(b, off)
	}

	return p.dPiece.WriteAt(b, off)
}

func (p *Piece) ReadAt(b []byte, off int64) (n int, err error) {
	if !settings.BTsets.UseDisk {
		return p.mPiece.ReadAt(b, off)
	}

	return p.dPiece.ReadAt(b, off)
}

func (p *Piece) MarkComplete() error {
	p.Complete = true

	return nil
}

func (p *Piece) MarkNotComplete() error {
	p.Complete = false

	return nil
}

func (p *Piece) Completion() storage.Completion {
	return storage.Completion{
		Complete: p.Complete,
		Ok:       true,
	}
}

func (p *Piece) Release() {
	if settings.BTsets.UseDisk {
		p.dPiece.Release()
	} else {
		p.mPiece.Release()
	}

	if p.cache != nil && !p.cache.isClosed.Load() {
		p.cache.torrent.Piece(p.ID).SetPriority(torrent.PiecePriorityNone)
		p.cache.torrent.Piece(p.ID).UpdateCompletion()
	}
}

// markAccessed updates LRU position and Accessed timestamp.
// Called from mempiece/diskpiece on read/write.
func (p *Piece) markAccessed() {
	if p.cache == nil {
		return
	}

	p.cache.lruMu.Lock()
	p.cache.markUsedLRU(p)
	p.cache.lruMu.Unlock()

	// Also update Accessed for backward compatibility with sort fallback
	p.Accessed = time.Now().Unix()
}
