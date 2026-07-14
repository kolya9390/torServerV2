package torrstor

import (
	"io"
	"sync"
	"testing"
)

const benchmarkPieceLength = int64(256 << 10)

type benchmarkMemPiece interface {
	WriteAt([]byte, int64) (int, error)
	ReadAt([]byte, int64) (int, error)
	Release()
}

type contiguousBenchmarkPiece struct {
	mu       sync.RWMutex
	pieceLen int64
	buf      []byte
}

func newContiguousBenchmarkPiece(pieceLen int64) *contiguousBenchmarkPiece {
	return &contiguousBenchmarkPiece{pieceLen: pieceLen}
}

func (p *contiguousBenchmarkPiece) WriteAt(data []byte, off int64) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(data) == 0 {
		return 0, nil
	}

	if off < 0 || off >= p.pieceLen {
		return 0, io.EOF
	}

	if p.buf == nil {
		p.buf = make([]byte, p.pieceLen)
	}

	return copy(p.buf[off:], data), nil
}

func (p *contiguousBenchmarkPiece) ReadAt(dst []byte, off int64) (int, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(dst) == 0 {
		return 0, nil
	}

	if p.buf == nil || off < 0 || off >= p.pieceLen {
		return 0, io.EOF
	}

	n := copy(dst, p.buf[off:])
	if n < len(dst) && off+int64(n) < p.pieceLen {
		return n, io.EOF
	}

	return n, nil
}

func (p *contiguousBenchmarkPiece) Release() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.buf = nil
}

func BenchmarkMemPieceComparison(b *testing.B) {
	payload := benchmarkPayload(benchmarkPieceLength)
	readBuf := make([]byte, benchmarkPieceLength)

	b.Run("sequential_write_read/chunked", func(b *testing.B) {
		piece := newBenchmarkChunkedPiece(b, benchmarkPieceLength)
		benchmarkSequentialWriteRead(b, piece, payload, readBuf)
	})

	b.Run("sequential_write_read/contiguous", func(b *testing.B) {
		piece := newContiguousBenchmarkPiece(benchmarkPieceLength)
		benchmarkSequentialWriteRead(b, piece, payload, readBuf)
	})

	b.Run("sparse_partial_read/chunked", func(b *testing.B) {
		piece := newBenchmarkChunkedPiece(b, benchmarkPieceLength)
		benchmarkSparsePartialRead(b, piece)
	})

	b.Run("sparse_partial_read/contiguous", func(b *testing.B) {
		piece := newContiguousBenchmarkPiece(benchmarkPieceLength)
		benchmarkSparsePartialRead(b, piece)
	})

	b.Run("release_churn/chunked", func(b *testing.B) {
		piece := newBenchmarkChunkedPiece(b, benchmarkPieceLength)
		benchmarkReleaseChurn(b, piece)
	})

	b.Run("release_churn/contiguous", func(b *testing.B) {
		piece := newContiguousBenchmarkPiece(benchmarkPieceLength)
		benchmarkReleaseChurn(b, piece)
	})

	b.Run("two_reader_concurrent/chunked", func(b *testing.B) {
		piece := newBenchmarkChunkedPiece(b, benchmarkPieceLength)
		benchmarkTwoReaderConcurrent(b, piece, payload)
	})

	b.Run("two_reader_concurrent/contiguous", func(b *testing.B) {
		piece := newContiguousBenchmarkPiece(benchmarkPieceLength)
		benchmarkTwoReaderConcurrent(b, piece, payload)
	})
}

func newBenchmarkChunkedPiece(b *testing.B, pieceLen int64) *MemPiece {
	b.Helper()
	drainMemPieceChunkPoolForBenchmark()

	cache := setupBenchmarkCache(pieceLen, 4)
	piece := cache.pieces[0]
	if piece == nil || piece.mPiece == nil {
		b.Fatal("benchmark mem piece is not initialized")
	}

	return piece.mPiece
}

func benchmarkPayload(size int64) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i)
	}

	return data
}

func benchmarkSequentialWriteRead(b *testing.B, piece benchmarkMemPiece, payload, readBuf []byte) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)) * 2)
	b.ResetTimer()

	for range b.N {
		piece.Release()

		if _, err := piece.WriteAt(payload, 0); err != nil {
			b.Fatal(err)
		}

		if _, err := piece.ReadAt(readBuf, 0); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkSparsePartialRead(b *testing.B, piece benchmarkMemPiece) {
	b.Helper()

	head := benchmarkPayload(memPieceChunkSize)
	tail := benchmarkPayload(memPieceChunkSize)
	readBuf := make([]byte, benchmarkPieceLength)
	tailOff := benchmarkPieceLength - int64(len(tail))

	b.ReportAllocs()
	b.SetBytes(int64(len(head)+len(tail)) + int64(len(readBuf)))
	b.ResetTimer()

	for range b.N {
		piece.Release()

		if _, err := piece.WriteAt(head, 0); err != nil {
			b.Fatal(err)
		}

		if _, err := piece.WriteAt(tail, tailOff); err != nil {
			b.Fatal(err)
		}

		if _, err := piece.ReadAt(readBuf, 0); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkReleaseChurn(b *testing.B, piece benchmarkMemPiece) {
	b.Helper()

	payload := benchmarkPayload(32 << 10)

	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()

	for range b.N {
		if _, err := piece.WriteAt(payload, 0); err != nil {
			b.Fatal(err)
		}

		piece.Release()
	}
}

func benchmarkTwoReaderConcurrent(b *testing.B, piece benchmarkMemPiece, payload []byte) {
	b.Helper()

	readBuf := make([]byte, benchmarkPieceLength)

	if _, err := piece.WriteAt(payload, 0); err != nil {
		b.Fatal(err)
	}
	defer piece.Release()

	b.ReportAllocs()
	b.SetBytes(int64(len(readBuf)) * 2)
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		localBuf := make([]byte, len(readBuf))
		for pb.Next() {
			if _, err := piece.ReadAt(localBuf, 0); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func drainMemPieceChunkPoolForBenchmark() {
	for {
		select {
		case <-memPieceChunkPool:
			memPiecePooledChunks.Add(-1)
		default:
			return
		}
	}
}
