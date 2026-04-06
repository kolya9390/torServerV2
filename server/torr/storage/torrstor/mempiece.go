package torrstor

import (
	"io"
	"sync"
)

// memPieceBufPool reuses byte slices for piece buffers.
// Buffers are sized to common torrent piece lengths (16KB–4MB).
// Get() returns a buffer >= requested size; Put() returns it to the pool.
var memPieceBufPool = sync.Pool{
	New: func() any {
		return new([]byte)
	},
}

func getBuffer(size int) []byte {
	ptr := memPieceBufPool.Get().(*[]byte)

	buf := *ptr
	if cap(buf) < size {
		// Grow buffer to avoid reallocation on every write
		buf = make([]byte, size)
	}

	return buf[:size]
}

func putBuffer(buf []byte) {
	// Don't pool tiny buffers — not worth the GC overhead
	if cap(buf) < 4096 {
		return
	}

	ptr := memPieceBufPool.Get().(*[]byte)
	if cap(buf) > cap(*ptr) {
		// Replace pooled buffer with larger one
		*ptr = buf
	}

	memPieceBufPool.Put(ptr)
}

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
			defer func() { _ = recover() }()
			p.piece.cache.cleanPieces()
		}()

		pieceLen := int(p.piece.cache.pieceLength)
		p.buffer = getBuffer(pieceLen)
	}

	n = copy(p.buffer[off:], b[:])

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

	size := len(b)
	if size+int(off) > len(p.buffer) {
		size = max(len(p.buffer)-int(off), 0)
	}

	if len(p.buffer) < int(off) || len(p.buffer) < int(off)+size {
		return 0, io.EOF
	}

	n = copy(b, p.buffer[int(off) : int(off)+size][:])
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
		putBuffer(p.buffer)
		p.buffer = nil
	}

	p.piece.Size.Store(0)
	p.piece.Complete = false
}
