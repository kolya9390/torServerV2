package torrstor

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"server/log"
	"server/settings"
)

type DiskPiece struct {
	piece *Piece

	name string

	mu sync.RWMutex
}

func NewDiskPiece(p *Piece) *DiskPiece {
	name := filepath.Join(settings.BTsets.TorrentsSavePath, p.cache.hash.HexString(), strconv.Itoa(p.Id))
	ff, err := os.Stat(name)
	if err == nil {
		p.WarmSize = ff.Size()
		p.Complete = ff.Size() == p.cache.pieceLength
		p.WarmAccessed = ff.ModTime().Unix()
	}
	return &DiskPiece{piece: p, name: name}
}

func (p *DiskPiece) WriteAt(b []byte, off int64) (n int, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.piece.cache.diskWriter != nil {
		n, err = p.piece.cache.diskWriter.WriteAt(p.name, b, off)
	} else {
		ff, openErr := os.OpenFile(p.name, os.O_RDWR|os.O_CREATE, 0o666)
		if openErr != nil {
			log.TLogln("Error open file:", openErr)
			return 0, openErr
		}
		defer func() {
			_ = ff.Close()
		}()
		n, err = ff.WriteAt(b, off)
	}

	p.piece.WarmSize += int64(n)
	if p.piece.WarmSize > p.piece.cache.pieceLength {
		p.piece.WarmSize = p.piece.cache.pieceLength
	}
	p.piece.WarmAccessed = time.Now().Unix()
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
	defer func() {
		_ = ff.Close()
	}()

	n, err = ff.ReadAt(b, off)
	if err != nil && err != io.EOF {
		return n, err
	}

	p.piece.WarmAccessed = time.Now().Unix()
	if int64(len(b))+off >= p.piece.WarmSize {
		p.piece.cache.scheduleCleanPieces()
	}
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

func (p *DiskPiece) Release() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.piece.WarmSize = 0

	_ = os.Remove(p.name)
}

func (p *DiskPiece) WriteFull(b []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	ff, err := os.OpenFile(p.name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		log.TLogln("Error open file:", err)
		return err
	}
	defer func() {
		_ = ff.Close()
	}()

	n, err := ff.WriteAt(b, 0)
	if err != nil {
		return err
	}
	p.piece.WarmSize = int64(n)
	if p.piece.WarmSize > p.piece.cache.pieceLength {
		p.piece.WarmSize = p.piece.cache.pieceLength
	}
	p.piece.WarmAccessed = time.Now().Unix()
	return nil
}
