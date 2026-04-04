package torrstor

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/anacrolix/torrent/metainfo"

	"server/settings"
)

func BenchmarkTieredCacheReadHotProfile(b *testing.B) {
	cache, piece := setupTieredBench(b)
	payload := []byte("tiered-cache-hot-read")
	_, _ = piece.WriteAt(payload, 0)
	_ = piece.MarkComplete()

	buf := make([]byte, len(payload))
	samples := make([]int64, 0, minIntBench(b.N, 4096))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		_, _ = piece.ReadAt(buf, 0)
		if i < cap(samples) {
			samples = append(samples, time.Since(start).Nanoseconds())
		}
	}
	b.StopTimer()
	reportBenchPercentiles(b, samples)
	_ = cache
}

func BenchmarkTieredCacheReadWarmProfile(b *testing.B) {
	_, piece := setupTieredBench(b)
	payload := []byte("tiered-cache-warm-read")
	_, _ = piece.WriteAt(payload, 0)
	_ = piece.MarkComplete()
	piece.Release()

	buf := make([]byte, len(payload))
	samples := make([]int64, 0, minIntBench(b.N, 4096))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		piece.Release() // keep reads on warm tier path
		start := time.Now()
		_, _ = piece.ReadAt(buf, 0)
		if i < cap(samples) {
			samples = append(samples, time.Since(start).Nanoseconds())
		}
	}
	b.StopTimer()
	reportBenchPercentiles(b, samples)
}

func setupTieredBench(tb testing.TB) (*Cache, *Piece) {
	tb.Helper()
	tmpDir := tb.TempDir()
	prev := settings.BTsets
	settings.BTsets = &settings.BTSets{
		UseDisk:             true,
		TorrentsSavePath:    tmpDir,
		WarmDiskCacheTTLMin: 180,
	}
	tb.Cleanup(func() {
		settings.BTsets = prev
	})

	cache := NewCache(8<<20, nil)
	cache.hash = metainfo.Hash{1, 2, 3, 4}
	cache.pieceLength = 128
	cache.pieces = make(map[int]*Piece)
	cache.readers = make(map[*Reader]struct{})
	cache.warmLimitBytes = 64 << 20
	cache.warmTTL = 3 * time.Hour
	if err := os.MkdirAll(filepath.Join(tmpDir, cache.hash.HexString()), 0o755); err != nil {
		tb.Fatalf("mkdir: %v", err)
	}
	piece := NewPiece(0, cache)
	cache.pieces[0] = piece
	return cache, piece
}

func reportBenchPercentiles(b *testing.B, samples []int64) {
	if len(samples) == 0 {
		return
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	b.ReportMetric(float64(percentileBench(samples, 50)), "p50_ns/op")
	b.ReportMetric(float64(percentileBench(samples, 95)), "p95_ns/op")
	b.ReportMetric(float64(percentileBench(samples, 99)), "p99_ns/op")
}

func percentileBench(samples []int64, p int) int64 {
	if len(samples) == 0 {
		return 0
	}
	if p <= 0 {
		return samples[0]
	}
	if p >= 100 {
		return samples[len(samples)-1]
	}
	idx := (len(samples) - 1) * p / 100
	return samples[idx]
}

func minIntBench(a, b int) int {
	if a < b {
		return a
	}
	return b
}
