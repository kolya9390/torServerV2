package torrstor

import (
	"bytes"
	"context"
	"testing"
	"time"

	"server/settings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

func setupStorageTest() {
	settings.DefaultSettingsProvider.Set(&settings.BTSets{
		CacheSize:        1 * 1024 * 1024,
		UseDisk:          false,
		TorrentsSavePath: "",
	})
}

func TestNewStorage(t *testing.T) {
	stor := NewStorage(64 * 1024 * 1024)
	if stor == nil {
		t.Fatal("NewStorage() returned nil")
	}

	if stor.manager == nil || stor.manager.capacity != 64*1024*1024 {
		t.Errorf("capacity = %d, want %d", stor.manager.capacity, 64*1024*1024)
	}

	if stor.manager == nil || stor.manager.registry.items == nil {
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
	numPieces := (1000 + info.PieceLength - 1) / info.PieceLength
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

func TestMemPieceWriteAt_TracksAllocatedChunks(t *testing.T) {
	setupStorageTest()

	stor := NewStorage(1 * 1024 * 1024)
	cache := NewCache(1*1024*1024, stor)

	info := &metainfo.Info{
		Files:       []metainfo.FileInfo{{Path: []string{"test.bin"}, Length: 64 * 1024}},
		PieceLength: 64 * 1024,
	}
	info.Pieces = make([]byte, 20)
	hash := metainfo.NewHashFromHex("abcdef1234567890abcdef1234567890abcdef12")
	cache.Init(info, hash)

	piece := cache.pieces[0]
	data := bytes.Repeat([]byte{0xAB}, 1024)

	if _, err := piece.WriteAt(data, 0); err != nil {
		t.Fatalf("WriteAt first chunk error: %v", err)
	}

	if got, want := piece.Size.Load(), int64(memPieceChunkSize); got != want {
		t.Fatalf("piece.Size after first chunk = %d, want %d", got, want)
	}

	if _, err := piece.WriteAt(data, int64(memPieceChunkSize)); err != nil {
		t.Fatalf("WriteAt second chunk error: %v", err)
	}

	if got, want := piece.Size.Load(), int64(memPieceChunkSize*2); got != want {
		t.Fatalf("piece.Size after second chunk = %d, want %d", got, want)
	}
}

func TestMemPieceReadAt_MissingChunkReturnsZeros(t *testing.T) {
	setupStorageTest()

	stor := NewStorage(1 * 1024 * 1024)
	cache := NewCache(1*1024*1024, stor)

	info := &metainfo.Info{
		Files:       []metainfo.FileInfo{{Path: []string{"test.bin"}, Length: 64 * 1024}},
		PieceLength: 64 * 1024,
	}
	info.Pieces = make([]byte, 20)
	hash := metainfo.NewHashFromHex("abcdef1234567890abcdef1234567890abcdef12")
	cache.Init(info, hash)

	piece := cache.pieces[0]
	data := bytes.Repeat([]byte{0xCD}, 1024)

	if _, err := piece.WriteAt(data, int64(memPieceChunkSize)); err != nil {
		t.Fatalf("WriteAt error: %v", err)
	}

	buf := make([]byte, 512)
	n, err := piece.ReadAt(buf, 0)
	if err != nil {
		t.Fatalf("ReadAt missing first chunk error = %v, want nil", err)
	}

	if n != len(buf) {
		t.Fatalf("ReadAt missing first chunk bytes = %d, want %d", n, len(buf))
	}

	if !bytes.Equal(buf, make([]byte, len(buf))) {
		t.Fatal("ReadAt missing first chunk should return zero-filled bytes")
	}
}

func TestMemPieceReadAt_SparseGapThenWrittenChunk(t *testing.T) {
	setupStorageTest()

	stor := NewStorage(1 * 1024 * 1024)
	cache := NewCache(1*1024*1024, stor)

	info := &metainfo.Info{
		Files:       []metainfo.FileInfo{{Path: []string{"test.bin"}, Length: 64 * 1024}},
		PieceLength: 64 * 1024,
	}
	info.Pieces = make([]byte, 20)
	hash := metainfo.NewHashFromHex("abcdef1234567890abcdef1234567890abcdef12")
	cache.Init(info, hash)

	piece := cache.pieces[0]
	data := bytes.Repeat([]byte{0xCD}, 1024)

	if _, err := piece.WriteAt(data, int64(memPieceChunkSize)); err != nil {
		t.Fatalf("WriteAt error: %v", err)
	}

	buf := make([]byte, memPieceChunkSize+len(data))
	n, err := piece.ReadAt(buf, 0)
	if err != nil {
		t.Fatalf("ReadAt sparse piece error = %v, want nil", err)
	}

	if n != len(buf) {
		t.Fatalf("ReadAt sparse piece bytes = %d, want %d", n, len(buf))
	}

	if !bytes.Equal(buf[:memPieceChunkSize], make([]byte, memPieceChunkSize)) {
		t.Fatal("leading sparse gap should be zero-filled")
	}

	if !bytes.Equal(buf[memPieceChunkSize:], data) {
		t.Fatal("written chunk tail mismatch")
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

	_, _ = stor.OpenTorrent(context.Background(), info, hash)

	if stor.cacheCount() != 1 {
		t.Errorf("caches count after Open = %d, want 1", stor.cacheCount())
	}

	stor.CloseHash(hash)

	if stor.cacheCount() != 0 {
		t.Errorf("caches count after CloseHash = %d, want 0", stor.cacheCount())
	}
}

func TestClearPriorityDelay(t *testing.T) {
	setupStorageTest()

	stor := NewStorage(1 * 1024 * 1024)
	cache := NewCache(1*1024*1024, stor)

	settings.DefaultSettingsProvider.Get().TorrentDisconnectTimeout = 0

	if got := cache.clearPriorityDelay(); got != time.Second {
		t.Fatalf("clearPriorityDelay() with timeout=0 = %v, want %v", got, time.Second)
	}

	settings.DefaultSettingsProvider.Get().TorrentDisconnectTimeout = 1

	if got := cache.clearPriorityDelay(); got != time.Second {
		t.Fatalf("clearPriorityDelay() with timeout=1 = %v, want %v", got, time.Second)
	}

	settings.DefaultSettingsProvider.Get().TorrentDisconnectTimeout = 10

	if got := cache.clearPriorityDelay(); got != time.Second {
		t.Fatalf("clearPriorityDelay() with timeout=10 = %v, want %v", got, time.Second)
	}

	settings.DefaultSettingsProvider.Get().TorrentDisconnectTimeout = 30

	if got := cache.clearPriorityDelay(); got != time.Second {
		t.Fatalf("clearPriorityDelay() with timeout=30 = %v, want %v", got, time.Second)
	}

	settings.DefaultSettingsProvider.Get().TorrentDisconnectTimeout = 120

	if got := cache.clearPriorityDelay(); got != time.Second {
		t.Fatalf("clearPriorityDelay() with timeout=120 = %v, want %v", got, time.Second)
	}
}

func TestSetCapacityClamp(t *testing.T) {
	setupStorageTest()

	stor := NewStorage(128 * 1024 * 1024)
	cache := NewCache(128*1024*1024, stor)
	cache.pieceLength = 4 << 20

	cache.SetCapacity(2 << 20)

	if got, want := cache.GetCapacity(), int64(8<<20); got != want {
		t.Fatalf("SetCapacity min clamp = %d, want %d", got, want)
	}

	cache.SetCapacity(64 << 20)

	if got, want := cache.GetCapacity(), int64(64<<20); got != want {
		t.Fatalf("SetCapacity regular update = %d, want %d", got, want)
	}
}

func TestPriorityPieceBudget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		connectionsLimit int
		activeReaders    int
		pieceLength      int64
		want             int
	}{
		{name: "single reader uses full connection budget", connectionsLimit: 25, activeReaders: 1, pieceLength: 4 << 20, want: 25},
		{name: "two readers split connection budget", connectionsLimit: 25, activeReaders: 2, pieceLength: 4 << 20, want: 12},
		{name: "three readers split connection budget", connectionsLimit: 6, activeReaders: 3, pieceLength: 4 << 20, want: 2},
		{name: "reader count floor", connectionsLimit: 4, activeReaders: 0, pieceLength: 1 << 20, want: 4},
		{name: "connection floor", connectionsLimit: 0, activeReaders: 3, pieceLength: 1 << 20, want: 1},
	}

	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := priorityPieceBudget(tt.connectionsLimit, tt.activeReaders, tt.pieceLength)
			if got != tt.want {
				t.Fatalf(
					"priorityPieceBudget(%d, %d, %d) = %d, want %d",
					tt.connectionsLimit,
					tt.activeReaders,
					tt.pieceLength,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestReaderOffsetRangeForReaders_UsesCapacityWindow(t *testing.T) {
	t.Parallel()

	settings.DefaultSettingsProvider.Set(&settings.BTSets{
		ReaderReadAHead: 95,
	})

	cache := &Cache{
		capacity:    128 << 20,
		pieceLength: 4 << 20,
	}
	reader := &Reader{
		cache: cache,
	}
	reader.offset.Store(64 << 20)

	begin, end := reader.getOffsetRangeForReaders(1)
	if got, want := end-reader.offset.Load(), int64((128<<20)*95/100); got != want {
		t.Fatalf("forward window = %d, want %d", got, want)
	}

	wantBack := int64((128 << 20) * 5 / 100)
	if got, want := reader.offset.Load()-begin, wantBack; got != want {
		t.Fatalf("back window = %d, want %d", got, want)
	}
}

func TestMaxPiecePriority(t *testing.T) {
	t.Parallel()

	if got := maxPiecePriority(torrent.PiecePriorityNormal, torrent.PiecePriorityHigh); got != torrent.PiecePriorityHigh {
		t.Fatalf("maxPiecePriority(normal, high) = %v, want %v", got, torrent.PiecePriorityHigh)
	}

	if got := maxPiecePriority(torrent.PiecePriorityNow, torrent.PiecePriorityHigh); got != torrent.PiecePriorityNow {
		t.Fatalf("maxPiecePriority(now, high) = %v, want %v", got, torrent.PiecePriorityNow)
	}
}

func TestDesiredPiecePriority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pieceID   int
		readerPos int
		readerRAH int
		wantPrio  torrent.PiecePriority
	}{
		{name: "current piece", pieceID: 10, readerPos: 10, readerRAH: 14, wantPrio: torrent.PiecePriorityNow},
		{name: "next piece", pieceID: 11, readerPos: 10, readerRAH: 14, wantPrio: torrent.PiecePriorityNext},
		{name: "readahead window", pieceID: 13, readerPos: 10, readerRAH: 14, wantPrio: torrent.PiecePriorityReadahead},
		{name: "high tail", pieceID: 18, readerPos: 10, readerRAH: 14, wantPrio: torrent.PiecePriorityHigh},
		{name: "normal tail", pieceID: 25, readerPos: 10, readerRAH: 14, wantPrio: torrent.PiecePriorityNormal},
	}

	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := desiredPiecePriority(tt.pieceID, tt.readerPos, tt.readerRAH); got != tt.wantPrio {
				t.Fatalf(
					"desiredPiecePriority(%d, %d, %d) = %v, want %v",
					tt.pieceID,
					tt.readerPos,
					tt.readerRAH,
					got,
					tt.wantPrio,
				)
			}
		})
	}
}
