package torrstor

import (
	"io"
	"sync"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"
	"server/settings"
)

type Piece struct {
	storage.PieceImpl `json:"-"`

	muAccessed sync.RWMutex

	Id   int   `json:"-"`
	Size int64 `json:"size"`
	// WarmSize is tier-2 disk cache footprint for the piece.
	WarmSize int64 `json:"-"`

	Complete     bool  `json:"complete"`
	Accessed     int64 `json:"accessed"`
	WarmAccessed int64 `json:"-"`

	mPiece *MemPiece  `json:"-"`
	dPiece *DiskPiece `json:"-"`

	cache *Cache `json:"-"`
}

func NewPiece(id int, cache *Cache) *Piece {
	p := &Piece{
		Id:    id,
		cache: cache,
	}

	if !settings.BTsets.UseDisk {
		p.mPiece = NewMemPiece(p)
	} else {
		p.mPiece = NewMemPiece(p)
		p.dPiece = NewDiskPiece(p)
	}
	return p
}

func (p *Piece) WriteAt(b []byte, off int64) (n int, err error) {
	if !settings.BTsets.UseDisk {
		return p.mPiece.WriteAt(b, off)
	}
	// In two-tier mode writes first land in RAM (hot cache).
	return p.mPiece.WriteAt(b, off)
}

func (p *Piece) ReadAt(b []byte, off int64) (n int, err error) {
	if !settings.BTsets.UseDisk {
		return p.mPiece.ReadAt(b, off)
	}

	// Tier-1 hot RAM read.
	n, err = p.mPiece.ReadAt(b, off)
	if n > 0 {
		p.cache.incHotHit()
		return n, err
	}

	// Tier-2 warm disk fallback.
	n, err = p.dPiece.ReadAt(b, off)
	if n > 0 {
		p.cache.incWarmHit()
		_, _ = p.mPiece.WriteAt(b[:n], off)
		return n, err
	}
	p.cache.incMiss()
	return 0, io.EOF
}

func (p *Piece) MarkComplete() error {
	p.Complete = true
	if settings.BTsets.UseDisk && p.dPiece != nil {
		if buf, ok := p.mPiece.CloneBuffer(); ok {
			_ = p.dPiece.WriteFull(buf)
		}
	}
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
	if p.mPiece != nil {
		p.mPiece.Release()
	}
	p.cache.incHotEviction()

	if !settings.BTsets.UseDisk || p.WarmSize == 0 {
		p.Complete = false
		if !p.cache.isClosed && p.cache.torrent != nil {
			p.cache.torrent.Piece(p.Id).SetPriority(torrent.PiecePriorityNone)
			p.cache.torrent.Piece(p.Id).UpdateCompletion()
		}
		return
	}

	if !p.cache.isClosed && p.cache.torrent != nil {
		p.cache.torrent.Piece(p.Id).SetPriority(torrent.PiecePriorityNone)
	}
}

func (p *Piece) ReleaseWarm() {
	if p.dPiece == nil {
		return
	}
	p.dPiece.Release()
	p.cache.incWarmEviction()
	if p.Size == 0 {
		p.Complete = false
		if !p.cache.isClosed && p.cache.torrent != nil {
			p.cache.torrent.Piece(p.Id).UpdateCompletion()
		}
	}
}

func (p *Piece) HotState() (size int64, accessed int64) {
	if p == nil {
		return 0, 0
	}
	if p.mPiece != nil {
		p.mPiece.mu.RLock()
		defer p.mPiece.mu.RUnlock()
	}
	p.muAccessed.RLock()
	defer p.muAccessed.RUnlock()
	return p.Size, p.Accessed
}
