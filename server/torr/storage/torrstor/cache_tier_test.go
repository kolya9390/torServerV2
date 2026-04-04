package torrstor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anacrolix/torrent/metainfo"

	"server/settings"
)

func setupTieredCacheTest(t *testing.T) (*Cache, *Piece, string) {
	t.Helper()
	tmpDir := t.TempDir()
	prev := settings.BTsets
	t.Cleanup(func() {
		settings.BTsets = prev
	})
	settings.BTsets = &settings.BTSets{
		UseDisk:             true,
		TorrentsSavePath:    tmpDir,
		WarmDiskCacheTTLMin: 180,
	}

	cache := NewCache(4<<20, nil)
	cache.hash = metainfo.Hash{1, 2, 3}
	cache.pieceLength = 64
	cache.pieces = make(map[int]*Piece)
	cache.readers = make(map[*Reader]struct{})
	cache.warmLimitBytes = 64 << 20
	cache.warmTTL = 3 * time.Hour
	if err := os.MkdirAll(filepath.Join(tmpDir, cache.hash.HexString()), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	p := NewPiece(0, cache)
	cache.pieces[0] = p
	return cache, p, tmpDir
}

func TestPieceReadFallsBackToWarmAndPromotesToHot(t *testing.T) {
	cache, p, _ := setupTieredCacheTest(t)

	payload := []byte("hello-tiered-cache")
	if _, err := p.WriteAt(payload, 0); err != nil {
		t.Fatalf("write hot: %v", err)
	}
	if err := p.MarkComplete(); err != nil {
		t.Fatalf("mark complete: %v", err)
	}
	p.Release() // evict hot only, keep warm tier

	buf := make([]byte, len(payload))
	n, err := p.ReadAt(buf, 0)
	if err != nil && n == 0 {
		t.Fatalf("read warm: %v", err)
	}
	if string(buf[:n]) != string(payload) {
		t.Fatalf("unexpected payload after warm read: got=%q", string(buf[:n]))
	}

	metrics := cache.Metrics()
	if metrics.WarmHits == 0 {
		t.Fatalf("expected warm hit metric > 0")
	}

	_, _ = p.ReadAt(buf, 0)
	metrics = cache.Metrics()
	if metrics.HotHits == 0 {
		t.Fatalf("expected hot hit metric > 0 after promotion")
	}
}

func TestWarmTTLEvictionRemovesOldWarmPiece(t *testing.T) {
	cache, p, _ := setupTieredCacheTest(t)
	cache.warmTTL = time.Minute

	payload := []byte("warm-ttl-piece")
	if _, err := p.WriteAt(payload, 0); err != nil {
		t.Fatalf("write hot: %v", err)
	}
	if err := p.MarkComplete(); err != nil {
		t.Fatalf("mark complete: %v", err)
	}
	p.WarmAccessed = time.Now().Add(-2 * time.Minute).Unix()
	p.Release()

	cache.cleanWarmPieces()
	if p.WarmSize != 0 {
		t.Fatalf("expected warm piece to be evicted by ttl")
	}
}
