package torrstor

import (
	"io"
	"sync"

	"server/log"
)

// MemPiece holds an in-memory buffer for a single torrent piece.
// We avoid sync.Pool here because torrent pieces are large (16KB–4MB)
// and have a long lifecycle per piece. Direct allocation + GC is more
// predictable and prevents memory retention after cache cleanup.
type MemPiece struct {
	piece *Piece

	buffer []byte
	mu     sync.RWMutex
}

func NewMemPiece(p *Piece) *MemPiece {
	return &MemPiece{piece: p}
}

func (p *MemPiece) WriteAt(b []byte, off int64) (n int, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.buffer == nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.TLogln("cleanPieces panic recovered in goroutine:", r)
				}
			}()

			p.piece.cache.cleanPieces()
		}()

		pieceLen := int(p.piece.cache.pieceLength)
		p.buffer = make([]byte, pieceLen)
	}

	n = copy(p.buffer[off:], b)
	p.piece.Size.Add(int64(n))

	if p.piece.Size.Load() > p.piece.cache.pieceLength {
		p.piece.Size.Store(p.piece.cache.pieceLength)
	}

	p.piece.markAccessed()

	return
}

func (p *MemPiece) ReadAt(b []byte, off int64) (n int, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.buffer == nil {
		return 0, io.EOF
	}

	size := len(b)
	if size+int(off) > len(p.buffer) {
		size = max(len(p.buffer)-int(off), 0)
	}

	if len(p.buffer) < int(off) || len(p.buffer) < int(off)+size {
		return 0, io.EOF
	}

	n = copy(b, p.buffer[int(off):int(off)+size])
	p.piece.markAccessed()

	if int64(len(b))+off >= p.piece.Size.Load() {
		go p.piece.cache.cleanPieces()
	}

	if n == 0 {
		return 0, io.EOF
	}

	return n, nil
}

func (p *MemPiece) Release() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.buffer != nil {
		// Directly nil out the buffer. Go's runtime will reclaim the
		// underlying memory on the next GC cycle without pooling overhead.
		p.buffer = nil
	}

	p.piece.Size.Store(0)
	p.piece.Complete = false
}
