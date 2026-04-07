package torrstor

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"server/log"
	"server/settings"
)

type DiskPiece struct {
	piece *Piece

	name string

	mu sync.RWMutex
}

func NewDiskPiece(p *Piece) *DiskPiece {
	name := filepath.Join(settings.BTsets.TorrentsSavePath, p.cache.hash.HexString(), strconv.Itoa(p.ID))

	ff, err := os.Stat(name)
	if err == nil {
		p.Size.Store(ff.Size())
		p.Complete = ff.Size() == p.cache.pieceLength
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

	n, err = ff.WriteAt(data, off)

	p.piece.Size.Add(int64(n))

	if p.piece.Size.Load() > p.piece.cache.pieceLength {
		p.piece.Size.Store(p.piece.cache.pieceLength)
	}

	p.piece.markAccessed()

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
		go p.piece.cache.cleanPieces()
	}

	return n, nil
}

func (p *DiskPiece) Release() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.piece.Size.Store(0)
	p.piece.Complete = false

	_ = os.Remove(p.name)
}
