package torrstor

import (
	"bytes"
	"testing"

	"server/settings"

	"github.com/anacrolix/torrent/metainfo"
)

func setupStorageTest() {
	settings.BTsets = &settings.BTSets{
		CacheSize:        1 * 1024 * 1024,
		UseDisk:          false,
		TorrentsSavePath: "",
	}
}

func TestNewStorage(t *testing.T) {
	stor := NewStorage(64 * 1024 * 1024)
	if stor == nil {
		t.Fatal("NewStorage() returned nil")
	}

	if stor.capacity != 64*1024*1024 {
		t.Errorf("capacity = %d, want %d", stor.capacity, 64*1024*1024)
	}

	if stor.caches == nil {
		t.Fatal("caches map not initialized")
	}
}

func TestNewCache(t *testing.T) {
	setupStorageTest()

	stor := NewStorage(32 * 1024 * 1024)

	cache := NewCache(32*1024*1024, stor)
	if cache == nil {
		t.Fatal("NewCache() returned nil")
	}

	if cache.capacity != 32*1024*1024 {
		t.Errorf("cache capacity = %d, want %d", cache.capacity, 32*1024*1024)
	}
}

func TestCacheInit(t *testing.T) {
	setupStorageTest()

	stor := NewStorage(64 * 1024 * 1024)
	cache := NewCache(64*1024*1024, stor)

	// Create a minimal torrent info for testing
	info := &metainfo.Info{
		Files: []metainfo.FileInfo{
			{
				Path:   []string{"test.txt"},
				Length: 1000,
			},
		},
		PieceLength: 16384, // 16 KB
	}
	// Calculate number of pieces
	numPieces := (1000+info.PieceLength-1) / info.PieceLength
	info.Pieces = make([]byte, numPieces*20)

	hash := metainfo.NewHashFromHex("abcdef1234567890abcdef1234567890abcdef12")
	cache.Init(info, hash)

	if cache.pieceCount != int(numPieces) {
		t.Errorf("pieceCount = %d, want %d", cache.pieceCount, numPieces)
	}

	if cache.pieceLength != info.PieceLength {
		t.Errorf("pieceLength = %d, want %d", cache.pieceLength, info.PieceLength)
	}

	if len(cache.pieces) != cache.pieceCount {
		t.Errorf("pieces map size = %d, want %d", len(cache.pieces), cache.pieceCount)
	}
}

func TestMemPieceWriteRead(t *testing.T) {
	setupStorageTest()

	stor := NewStorage(1 * 1024 * 1024)
	cache := NewCache(1*1024*1024, stor)

	info := &metainfo.Info{
		Files: []metainfo.FileInfo{
			{Path: []string{"test.bin"}, Length: 4096},
		},
		PieceLength: 4096,
	}
	info.Pieces = make([]byte, 20)
	hash := metainfo.NewHashFromHex("abcdef1234567890abcdef1234567890abcdef12")
	cache.Init(info, hash)

	// Write to piece
	piece := cache.pieces[0]
	if piece == nil {
		t.Fatal("piece[0] is nil")
	}

	data := []byte("Hello, Torrent!")

	n, err := piece.WriteAt(data, 0)
	if err != nil {
		t.Fatalf("WriteAt error: %v", err)
	}

	if n != len(data) {
		t.Errorf("WriteAt returned %d bytes, want %d", n, len(data))
	}

	// Read from piece
	buf := make([]byte, len(data))

	n, err = piece.ReadAt(buf, 0)
	if err != nil {
		t.Fatalf("ReadAt error: %v", err)
	}

	if n != len(data) {
		t.Errorf("ReadAt returned %d bytes, want %d", n, len(data))
	}

	if !bytes.Equal(buf, data) {
		t.Errorf("ReadAt data = %q, want %q", buf, data)
	}
}

func TestPieceFake(t *testing.T) {
	fake := &PieceFake{}
	buf := make([]byte, 10)

	_, err := fake.ReadAt(buf, 0)
	if err == nil {
		t.Error("PieceFake.ReadAt should return error")
	}

	_, err = fake.WriteAt(buf, 0)
	if err == nil {
		t.Error("PieceFake.WriteAt should return error")
	}
}

func TestRanges(t *testing.T) {
	ranges := []Range{
		{Start: 0, End: 10},
		{Start: 20, End: 30},
	}

	if !inRanges(ranges, 5) {
		t.Error("5 should be in ranges")
	}

	if !inRanges(ranges, 25) {
		t.Error("25 should be in ranges")
	}

	if inRanges(ranges, 15) {
		t.Error("15 should not be in ranges")
	}
}

func TestMergeRanges(t *testing.T) {
	ranges := []Range{
		{Start: 0, End: 10},
		{Start: 5, End: 15},
		{Start: 20, End: 30},
	}

	merged := mergeRange(ranges)
	if len(merged) != 2 {
		t.Errorf("merged ranges count = %d, want 2", len(merged))
	}
}

func TestStorageCloseHash(t *testing.T) {
	setupStorageTest()

	stor := NewStorage(1 * 1024 * 1024)
	info := &metainfo.Info{
		Files:       []metainfo.FileInfo{{Path: []string{"test.bin"}, Length: 100}},
		PieceLength: 100,
	}
	info.Pieces = make([]byte, 20)
	hash := metainfo.NewHashFromHex("abcdef1234567890abcdef1234567890abcdef12")

	_, _ = stor.OpenTorrent(info, hash)

	if len(stor.caches) != 1 {
		t.Errorf("caches count after Open = %d, want 1", len(stor.caches))
	}

	stor.CloseHash(hash)

	if len(stor.caches) != 0 {
		t.Errorf("caches count after CloseHash = %d, want 0", len(stor.caches))
	}
}
