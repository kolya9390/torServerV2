package torrstor

import (
	"io"
	"sync"
	"sync/atomic"
)

const (
	memPieceChunkSize      = 16 << 10
	memPieceChunkPoolLimit = 4096
)

var (
	memPieceChunkPool    = make(chan []byte, memPieceChunkPoolLimit)
	memPiecePooledChunks atomic.Int64
)

// MemPiece holds an in-memory buffer for a single torrent piece.
// Data is stored in 16 KiB chunks to match the default anacrolix/torrent
// request size. This avoids allocating a full piece buffer on the first
// partial write and keeps cache accounting closer to real residency.
type MemPiece struct {
	piece *Piece

	chunks [][]byte
	mu     sync.RWMutex
}

func NewMemPiece(p *Piece) *MemPiece {
	return &MemPiece{piece: p}
}

func (p *MemPiece) pieceLength() int64 {
	return p.piece.cache.pieceLength
}

func (p *MemPiece) chunkLen(index int) int {
	chunkStart := int64(index) * memPieceChunkSize

	remaining := p.pieceLength() - chunkStart
	if remaining <= 0 {
		return 0
	}

	if remaining < memPieceChunkSize {
		return int(remaining)
	}

	return memPieceChunkSize
}

func (p *MemPiece) ensureChunks() {
	if p.chunks != nil {
		return
	}

	pieceLen := p.pieceLength()

	chunks := int((pieceLen + memPieceChunkSize - 1) / memPieceChunkSize)
	if chunks < 1 {
		chunks = 1
	}

	p.chunks = make([][]byte, chunks)
}

func getMemPieceChunk() []byte {
	select {
	case chunk := <-memPieceChunkPool:
		memPiecePooledChunks.Add(-1)

		return chunk[:memPieceChunkSize]
	default:
		return make([]byte, memPieceChunkSize)
	}
}

func putMemPieceChunk(chunk []byte) {
	if cap(chunk) < memPieceChunkSize {
		return
	}

	chunk = chunk[:memPieceChunkSize]

	select {
	case memPieceChunkPool <- chunk:
		memPiecePooledChunks.Add(1)
	default:
	}
}

func reusableMemPieceChunks() int64 {
	return memPiecePooledChunks.Load()
}

func trimReusableMemPieceChunks(maxChunks int64) {
	if maxChunks < 0 {
		maxChunks = 0
	}

	for memPiecePooledChunks.Load() > maxChunks {
		select {
		case <-memPieceChunkPool:
			memPiecePooledChunks.Add(-1)
		default:
			return
		}
	}
}

func (p *MemPiece) WriteAt(b []byte, off int64) (n int, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(b) == 0 {
		return 0, nil
	}

	p.ensureChunks()

	allocated := int64(0)

	for len(b) > 0 {
		chunkIdx := int(off / memPieceChunkSize)
		if chunkIdx < 0 || chunkIdx >= len(p.chunks) {
			break
		}

		chunkOff := int(off % memPieceChunkSize)

		chunk := p.chunks[chunkIdx]
		if chunk == nil {
			chunkLen := p.chunkLen(chunkIdx)
			if chunkLen == memPieceChunkSize {
				chunk = getMemPieceChunk()
				if chunkOff != 0 || len(b) < chunkLen {
					clear(chunk)
				}
			} else {
				chunk = make([]byte, chunkLen)
			}

			chunk = chunk[:chunkLen]
			p.chunks[chunkIdx] = chunk
			p.piece.cache.addInMemoryChunks(1)

			allocated += int64(len(chunk))
		}

		written := copy(chunk[chunkOff:], b)
		n += written
		off += int64(written)
		b = b[written:]
	}

	if allocated > 0 {
		oldSize := p.piece.Size.Load()
		p.piece.Size.Store(oldSize + allocated)
		p.piece.cache.addFilled(allocated)

		if oldSize == 0 {
			p.piece.cache.markResidentPiece(p.piece)
		}

		// Queue cleanup asynchronously to avoid blocking the hot write path.
		p.piece.cache.queueCleanPieces()
	}

	p.piece.markAccessed()

	return
}

func (p *MemPiece) ReadAt(b []byte, off int64) (n int, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(b) == 0 {
		return 0, nil
	}

	if p.chunks == nil {
		return 0, io.EOF
	}

	pieceLen := p.pieceLength()
	if off < 0 || off >= pieceLen {
		return 0, io.EOF
	}

	remaining := min(int64(len(b)), pieceLen-off)
	startOff := off

	for remaining > 0 {
		chunkIdx := int(off / memPieceChunkSize)
		if chunkIdx < 0 || chunkIdx >= len(p.chunks) {
			break
		}

		chunk := p.chunks[chunkIdx]
		if chunk == nil {
			chunkOff := int(off % memPieceChunkSize)
			chunkLen := p.chunkLen(chunkIdx)

			if chunkOff >= chunkLen {
				break
			}

			// Keep sparse in-memory pieces readable like the original TorrServer
			// full-piece buffer: unread holes behave as zero-filled bytes instead
			// of surfacing EOF mid-piece. This avoids spurious playback failures
			// when the torrent library probes a piece before all its chunks are
			// physically written into cache.
			toCopy := min(int64(chunkLen-chunkOff), remaining)
			clear(b[n : n+int(toCopy)])
			n += int(toCopy)
			off += toCopy
			remaining -= toCopy

			continue
		}

		chunkOff := int(off % memPieceChunkSize)
		if chunkOff >= len(chunk) {
			break
		}

		toCopy := min(int64(len(chunk)-chunkOff), remaining)
		copied := copy(b[n:n+int(toCopy)], chunk[chunkOff:chunkOff+int(toCopy)])
		n += copied
		off += int64(copied)
		remaining -= int64(copied)

		if copied == 0 {
			break
		}
	}

	p.piece.markAccessed()

	if startOff+int64(n) >= pieceLen {
		p.piece.cache.queueCleanPieces()
	}

	if remaining > 0 {
		return n, io.EOF
	}

	return n, nil
}

func (p *MemPiece) Release() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.chunks != nil {
		releasedChunks := int64(0)

		for _, chunk := range p.chunks {
			if len(chunk) == memPieceChunkSize {
				putMemPieceChunk(chunk)
			}

			if chunk != nil {
				releasedChunks++
			}
		}

		p.chunks = nil
		p.piece.cache.addInMemoryChunks(-releasedChunks)
	}

	prev := p.piece.Size.Swap(0)
	p.piece.cache.addFilled(-prev)

	if prev > 0 {
		p.piece.cache.unmarkResidentPiece(p.piece)
	}

	p.piece.Complete = false
}
