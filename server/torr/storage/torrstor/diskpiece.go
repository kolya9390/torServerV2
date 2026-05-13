package torrstor

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"server/log"
)

type DiskPiece struct {
	piece *Piece

	name string

	mu sync.RWMutex
}

func NewDiskPiece(p *Piece) *DiskPiece {
	name := filepath.Join(p.cache.currentCacheConfig().SavePath, p.cache.hash.HexString(), strconv.Itoa(p.ID))

	ff, err := os.Stat(name)
	if err == nil {
		size := ff.Size()
		if size > p.cache.pieceLength {
			size = p.cache.pieceLength
		}

		p.Size.Store(size)
		p.cache.addFilled(size)
		p.Complete = size == p.cache.pieceLength
		p.Accessed = ff.ModTime().Unix()
	}

	return &DiskPiece{piece: p, name: name}
}

func (p *DiskPiece) WriteAt(data []byte, off int64) (n int, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	ff, err := os.OpenFile(p.name, os.O_RDWR|os.O_CREATE, 0o666)
	if err != nil {
		log.TLogln("Error open file:", err)

		return 0, err
	}

	defer func() { _ = ff.Close() }()

	// Check if this is first write (file is new/empty)
	stat, _ := ff.Stat()
	isFirstWrite := stat == nil || stat.Size() == 0

	n, err = ff.WriteAt(data, off)
	if n > 0 {
		oldSize := p.piece.Size.Load()

		newSize := oldSize + int64(n)
		if newSize > p.piece.cache.pieceLength {
			newSize = p.piece.cache.pieceLength
		}

		p.piece.Size.Store(newSize)
		p.piece.cache.addFilled(newSize - oldSize)
	}

	// Synchronous cleanup for immediate eviction on first write
	if isFirstWrite && err == nil {
		p.piece.markAccessed()
		p.piece.cache.CleanPieces()
	} else {
		p.piece.markAccessed()
	}

	return
}

func (p *DiskPiece) ReadAt(b []byte, off int64) (n int, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	ff, err := os.OpenFile(p.name, os.O_RDONLY, 0o666)
	if os.IsNotExist(err) {
		return 0, io.EOF
	}

	if err != nil {
		log.TLogln("Error open file:", err)

		return 0, err
	}

	defer func() { _ = ff.Close() }()

	n, err = ff.ReadAt(b, off)

	p.piece.markAccessed()

	if int64(len(b))+off >= p.piece.Size.Load() {
		p.piece.cache.queueCleanPieces()
	}

	return n, nil
}

func (p *DiskPiece) Release() {
	p.mu.Lock()
	defer p.mu.Unlock()

	prev := p.piece.Size.Swap(0)
	p.piece.cache.addFilled(-prev)
	p.piece.Complete = false

	_ = os.Remove(p.name)
}
