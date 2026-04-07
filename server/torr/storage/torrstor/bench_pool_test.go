package torrstor

import (
	"testing"

	"server/settings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

func setupBenchmarkCache(pieceLength int64, pieceCount int) *Cache {
	settings.BTsets = &settings.BTSets{
		CacheSize:        pieceLength * int64(pieceCount),
		UseDisk:          false,
		TorrentsSavePath: "",
	}
	stor := NewStorage(pieceLength * int64(pieceCount))
	cache := NewCache(pieceLength*int64(pieceCount), stor)

	// Build real metainfo.Info with dummy pieces
	info := &metainfo.Info{
		PieceLength: pieceLength,
		Pieces:      make([]byte, pieceCount*20),
	}
	cache.Init(info, [20]byte{1, 2, 3})

	// Set a minimal mock torrent to avoid nil pointer in clearPriority
	t := &torrent.Torrent{}
	cache.SetTorrent(t)

	return cache
}

func BenchmarkMemPieceWriteAt(b *testing.B) {
	// Standard torrent piece size: 256 KB
	pieceLen := int64(256 * 1024)
	cache := setupBenchmarkCache(pieceLen, 100)

	data := make([]byte, pieceLen)
	for i := range data {
		data[i] = byte(i % 256)
	}

	piece := cache.pieces[0]
	mp := piece.mPiece

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		// Simulate sequential piece write
		mp.Release()               // Reset to measure fresh allocation
		_, _ = mp.WriteAt(data, 0) //nolint:errcheck // benchmark intentionally ignores return value
	}
}

func BenchmarkMemPieceWriteAtSmallChunks(b *testing.B) {
	// Simulate torrent library writing in 32KB chunks
	pieceLen := int64(256 * 1024)
	chunkSize := int64(32 * 1024)
	cache := setupBenchmarkCache(pieceLen, 100)

	data := make([]byte, chunkSize)
	for i := range data {
		data[i] = byte(i % 256)
	}

	piece := cache.pieces[0]
	mp := piece.mPiece

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		mp.Release()
		// Write piece in 8 chunks of 32KB
		for off := int64(0); off < pieceLen; off += chunkSize {
			_, _ = mp.WriteAt(data, off) //nolint:errcheck // benchmark intentionally ignores return value
		}
	}
}

func BenchmarkMemPieceReadAt(b *testing.B) {
	pieceLen := int64(256 * 1024)
	cache := setupBenchmarkCache(pieceLen, 100)

	// Pre-fill piece
	piece := cache.pieces[0]

	data := make([]byte, pieceLen)
	for i := range data {
		data[i] = byte(i % 256)
	}

	_, _ = piece.mPiece.WriteAt(data, 0)

	buf := make([]byte, pieceLen)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, _ = piece.mPiece.ReadAt(buf, 0)
	}
}
