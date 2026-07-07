package torrstor

import (
	"bytes"
	"context"
	"sync"
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

type testCacheHost struct {
	sets *settings.BTSets
}

func (h testCacheHost) currentSettings() *settings.BTSets {
	return h.sets
}

func (testCacheHost) unregisterCache(metainfo.Hash) {}

func drainMemPieceChunkPoolForTest(t *testing.T) {
	t.Helper()

	for {
		select {
		case <-memPieceChunkPool:
			memPiecePooledChunks.Add(-1)
		default:
			return
		}
	}
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

func TestMemPieceWriteAt_TracksResidentPiece(t *testing.T) {
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

	before := SnapshotCacheStats()
	piece := cache.pieces[0]
	data := bytes.Repeat([]byte{0xAB}, 1024)

	if _, err := piece.WriteAt(data, 0); err != nil {
		t.Fatalf("WriteAt error: %v", err)
	}

	if got := len(cache.copyResidentPieces()); got != 1 {
		t.Fatalf("resident pieces after write = %d, want 1", got)
	}

	if got, want := SnapshotCacheStats().ResidentPieces-before.ResidentPieces, int64(1); got != want {
		t.Fatalf("resident pieces metric after write = %d, want %d", got, want)
	}

	piece.Release()

	if got := len(cache.copyResidentPieces()); got != 0 {
		t.Fatalf("resident pieces after release = %d, want 0", got)
	}

	if got, want := SnapshotCacheStats().ResidentPieces-before.ResidentPieces, int64(0); got != want {
		t.Fatalf("resident pieces metric after release = %d, want %d", got, want)
	}
}

func TestMarkResidentPieceSkipsClosedResidentIndex(t *testing.T) {
	setupStorageTest()

	cache := NewCache(1*1024*1024, testCacheHost{})
	info := &metainfo.Info{
		Files:       []metainfo.FileInfo{{Path: []string{"test.bin"}, Length: 64 * 1024}},
		PieceLength: 64 * 1024,
	}
	info.Pieces = make([]byte, 20)
	hash := metainfo.NewHashFromHex("abcdef1234567890abcdef1234567890abcdef12")
	cache.Init(info, hash)

	cache.resident.items = nil
	before := SnapshotCacheStats()

	cache.markResidentPiece(&Piece{ID: 1, cache: cache})

	if got := len(cache.copyResidentPieces()); got != 0 {
		t.Fatalf("resident pieces after closed index mark = %d, want 0", got)
	}

	if got, want := SnapshotCacheStats().ResidentPieces-before.ResidentPieces, int64(0); got != want {
		t.Fatalf("resident pieces metric after closed index mark = %d, want %d", got, want)
	}
}

func TestMemPieceReleaseClearsChunksAndAccounting(t *testing.T) {
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

	before := SnapshotCacheStats()
	piece := cache.pieces[0]
	data := bytes.Repeat([]byte{0xAB}, memPieceChunkSize+1024)

	if _, err := piece.WriteAt(data, 0); err != nil {
		t.Fatalf("WriteAt error: %v", err)
	}

	if piece.mPiece.chunks == nil {
		t.Fatal("chunks were not allocated")
	}

	afterWrite := SnapshotCacheStats()
	if got, want := afterWrite.InMemoryChunks-before.InMemoryChunks, int64(2); got != want {
		t.Fatalf("in-memory chunks delta after WriteAt = %d, want %d", got, want)
	}

	piece.mPiece.Release()

	if piece.mPiece.chunks != nil {
		t.Fatal("chunks were not cleared after Release")
	}

	afterRelease := SnapshotCacheStats()
	if got, want := afterRelease.InMemoryChunks-before.InMemoryChunks, int64(0); got != want {
		t.Fatalf("in-memory chunks delta after Release = %d, want %d", got, want)
	}

	if got, want := afterRelease.LogicalFilledBytes-before.LogicalFilledBytes, int64(0); got != want {
		t.Fatalf("logical filled delta after Release = %d, want %d", got, want)
	}
}

func TestMemPieceChunkPoolIsBounded(t *testing.T) {
	drainMemPieceChunkPoolForTest(t)

	for range memPieceChunkPoolLimit + 8 {
		putMemPieceChunk(make([]byte, memPieceChunkSize))
	}

	if got, want := reusableMemPieceChunks(), int64(memPieceChunkPoolLimit); got != want {
		t.Fatalf("reusable chunks = %d, want %d", got, want)
	}

	for range memPieceChunkPoolLimit {
		chunk := getMemPieceChunk()
		if len(chunk) != memPieceChunkSize {
			t.Fatalf("chunk len = %d, want %d", len(chunk), memPieceChunkSize)
		}
	}

	if got, want := reusableMemPieceChunks(), int64(0); got != want {
		t.Fatalf("reusable chunks after drain = %d, want %d", got, want)
	}
}

func TestCacheStatsTrackLogicalOverheadForTwoCaches(t *testing.T) {
	setupStorageTest()

	before := SnapshotCacheStats()
	stor := NewStorage(64 * 1024)

	first := NewCache(64*1024, stor)
	second := NewCache(64*1024, stor)

	info := &metainfo.Info{
		Files:       []metainfo.FileInfo{{Path: []string{"test.bin"}, Length: 256 * 1024}},
		PieceLength: 64 * 1024,
	}
	info.Pieces = make([]byte, 4*20)

	first.Init(info, metainfo.NewHashFromHex("abcdef1234567890abcdef1234567890abcdef12"))
	second.Init(info, metainfo.NewHashFromHex("1234567890abcdef1234567890abcdef12345678"))
	t.Cleanup(func() {
		_ = first.Close()
		_ = second.Close()
	})

	writePiece := func(t *testing.T, cache *Cache, id int) {
		t.Helper()

		data := bytes.Repeat([]byte{0xAB}, 64*1024)
		if _, err := cache.pieces[id].WriteAt(data, 0); err != nil {
			t.Fatalf("WriteAt cache piece %d error: %v", id, err)
		}
	}

	for i := range 2 {
		writePiece(t, first, i)
		writePiece(t, second, i)
	}

	afterWrite := SnapshotCacheStats()
	if got, want := afterWrite.ConfiguredCapacityBytes-before.ConfiguredCapacityBytes, int64(128*1024); got != want {
		t.Fatalf("configured capacity delta = %d, want %d", got, want)
	}

	if got, want := afterWrite.LogicalFilledBytes-before.LogicalFilledBytes, int64(256*1024); got != want {
		t.Fatalf("logical filled delta = %d, want %d", got, want)
	}

	logicalDelta := afterWrite.LogicalFilledBytes - before.LogicalFilledBytes
	capacityDelta := afterWrite.ConfiguredCapacityBytes - before.ConfiguredCapacityBytes
	if got, want := logicalDelta-capacityDelta, int64(128*1024); got != want {
		t.Fatalf("logical overhead delta = %d, want %d", got, want)
	}

	first.pieces[0].mPiece.Release()
	second.pieces[0].mPiece.Release()

	afterClean := SnapshotCacheStats()
	logicalDelta = afterClean.LogicalFilledBytes - before.LogicalFilledBytes
	capacityDelta = afterClean.ConfiguredCapacityBytes - before.ConfiguredCapacityBytes
	if got, want := logicalDelta-capacityDelta, int64(0); got != want {
		t.Fatalf("logical overhead delta after releasing removable pieces = %d, want %d", got, want)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("first Close error: %v", err)
	}

	if err := second.Close(); err != nil {
		t.Fatalf("second Close error: %v", err)
	}
}

func TestReusableChunkPoolDrainsAfterCacheCloses(t *testing.T) {
	setupStorageTest()
	drainMemPieceChunkPoolForTest(t)

	stor := NewStorage(64 * 1024)
	first := NewCache(64*1024, stor)
	second := NewCache(64*1024, stor)

	info := &metainfo.Info{
		Files:       []metainfo.FileInfo{{Path: []string{"test.bin"}, Length: 64 * 1024}},
		PieceLength: 64 * 1024,
	}
	info.Pieces = make([]byte, 20)

	first.Init(info, metainfo.NewHashFromHex("abcdef1234567890abcdef1234567890abcdef12"))
	second.Init(info, metainfo.NewHashFromHex("1234567890abcdef1234567890abcdef12345678"))

	data := bytes.Repeat([]byte{0xCD}, memPieceChunkSize)
	if _, err := first.pieces[0].WriteAt(data, 0); err != nil {
		t.Fatalf("WriteAt error: %v", err)
	}

	first.pieces[0].mPiece.Release()

	if got, want := reusableMemPieceChunks(), int64(1); got != want {
		t.Fatalf("reusable chunks after Release = %d, want %d", got, want)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("first Close error: %v", err)
	}

	if got, want := reusableMemPieceChunks(), int64(0); got != want {
		t.Fatalf("reusable chunks after cache close = %d, want %d", got, want)
	}

	if err := second.Close(); err != nil {
		t.Fatalf("second Close error: %v", err)
	}

	if got, want := reusableMemPieceChunks(), int64(0); got != want {
		t.Fatalf("reusable chunks after last cache close = %d, want %d", got, want)
	}
}

func TestCacheCloseReleasesResidentMemPieceChunksAndTrimsPool(t *testing.T) {
	setupStorageTest()
	drainMemPieceChunkPoolForTest(t)

	before := SnapshotCacheStats()
	stor := NewStorage(64 * 1024)
	cache := NewCache(64*1024, stor)

	info := &metainfo.Info{
		Files:       []metainfo.FileInfo{{Path: []string{"test.bin"}, Length: 64 * 1024}},
		PieceLength: 64 * 1024,
	}
	info.Pieces = make([]byte, 20)

	cache.Init(info, metainfo.NewHashFromHex("abcdef1234567890abcdef1234567890abcdef12"))

	piece := cache.pieces[0]
	data := bytes.Repeat([]byte{0xCD}, memPieceChunkSize)
	if _, err := piece.WriteAt(data, 0); err != nil {
		t.Fatalf("WriteAt error: %v", err)
	}

	afterWrite := SnapshotCacheStats()
	if got, want := afterWrite.InMemoryChunks-before.InMemoryChunks, int64(1); got != want {
		t.Fatalf("in-memory chunks delta after WriteAt = %d, want %d", got, want)
	}

	if err := cache.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	if piece.mPiece.chunks != nil {
		t.Fatal("resident chunks were not released during cache close")
	}

	afterClose := SnapshotCacheStats()
	if got, want := afterClose.ActiveCaches-before.ActiveCaches, int64(0); got != want {
		t.Fatalf("active caches delta after Close = %d, want %d", got, want)
	}

	if got, want := afterClose.LogicalFilledBytes-before.LogicalFilledBytes, int64(0); got != want {
		t.Fatalf("logical filled delta after Close = %d, want %d", got, want)
	}

	if got, want := afterClose.InMemoryChunks-before.InMemoryChunks, int64(0); got != want {
		t.Fatalf("in-memory chunks delta after Close = %d, want %d", got, want)
	}

	if got, want := reusableMemPieceChunks(), int64(0); got != want {
		t.Fatalf("reusable chunks after Close = %d, want %d", got, want)
	}
}

func TestCacheCloseIsIdempotent(t *testing.T) {
	setupStorageTest()

	before := SnapshotCacheStats()
	stor := NewStorage(64 * 1024)
	cache := NewCache(64*1024, stor)

	info := &metainfo.Info{
		Files:       []metainfo.FileInfo{{Path: []string{"test.bin"}, Length: 64 * 1024}},
		PieceLength: 64 * 1024,
	}
	info.Pieces = make([]byte, 20)

	cache.Init(info, metainfo.NewHashFromHex("abcdef1234567890abcdef1234567890abcdef12"))

	if err := cache.Close(); err != nil {
		t.Fatalf("first Close error: %v", err)
	}

	afterFirstClose := SnapshotCacheStats()

	if err := cache.Close(); err != nil {
		t.Fatalf("second Close error: %v", err)
	}

	afterSecondClose := SnapshotCacheStats()
	if afterSecondClose != afterFirstClose {
		t.Fatalf("second Close changed cache stats: first=%+v second=%+v", afterFirstClose, afterSecondClose)
	}

	if got, want := afterSecondClose.ActiveCaches-before.ActiveCaches, int64(0); got != want {
		t.Fatalf("active caches delta after double Close = %d, want %d", got, want)
	}
}

func TestPieceReleaseDoesNotDependOnCacheClosedState(t *testing.T) {
	setupStorageTest()
	drainMemPieceChunkPoolForTest(t)

	before := SnapshotCacheStats()
	stor := NewStorage(64 * 1024)
	cache := NewCache(64*1024, stor)

	info := &metainfo.Info{
		Files:       []metainfo.FileInfo{{Path: []string{"test.bin"}, Length: 64 * 1024}},
		PieceLength: 64 * 1024,
	}
	info.Pieces = make([]byte, 20)

	cache.Init(info, metainfo.NewHashFromHex("abcdef1234567890abcdef1234567890abcdef12"))
	cache.isClosed.Store(true)

	piece := cache.pieces[0]
	if _, err := piece.WriteAt(bytes.Repeat([]byte{0xEF}, memPieceChunkSize), 0); err != nil {
		t.Fatalf("WriteAt error: %v", err)
	}

	piece.Release()

	if piece.mPiece.chunks != nil {
		t.Fatal("Release should clear memory chunks even when cache is marked closed")
	}

	afterRelease := SnapshotCacheStats()
	if got, want := afterRelease.InMemoryChunks-before.InMemoryChunks, int64(0); got != want {
		t.Fatalf("in-memory chunks delta after Release = %d, want %d", got, want)
	}

	if got, want := afterRelease.LogicalFilledBytes-before.LogicalFilledBytes, int64(0); got != want {
		t.Fatalf("logical filled delta after Release = %d, want %d", got, want)
	}
}

func TestReleasedPieceTorrentSyncDecisions(t *testing.T) {
	t.Parallel()

	if shouldClearReleasedPiecePriority(torrent.PiecePriorityNone) {
		t.Fatal("release should not clear an already-none torrent piece priority")
	}

	if !shouldClearReleasedPiecePriority(torrent.PiecePriorityNormal) {
		t.Fatal("release should clear non-none torrent piece priority")
	}

	if shouldRefreshReleasedPieceCompletion(false) {
		t.Fatal("release should not refresh torrent completion for an already-incomplete piece")
	}

	if !shouldRefreshReleasedPieceCompletion(true) {
		t.Fatal("release should refresh torrent completion after evicting a complete piece")
	}
}

func TestCleanPiecesConcurrentCallsAreSafe(t *testing.T) {
	setupStorageTest()
	drainMemPieceChunkPoolForTest(t)

	stor := NewStorage(1 * 1024 * 1024)
	cache := NewCache(1*1024*1024, stor)

	info := &metainfo.Info{
		Files:       []metainfo.FileInfo{{Path: []string{"test.bin"}, Length: 8 * memPieceChunkSize}},
		PieceLength: memPieceChunkSize,
	}
	info.Pieces = make([]byte, 8*20)

	cache.Init(info, metainfo.NewHashFromHex("abcdef1234567890abcdef1234567890abcdef12"))

	for i := range 8 {
		data := bytes.Repeat([]byte{byte(i)}, memPieceChunkSize)
		if _, err := cache.pieces[i].WriteAt(data, 0); err != nil {
			t.Fatalf("WriteAt piece %d error: %v", i, err)
		}
	}

	cache.mu.Lock()
	cache.capacity = 2 * memPieceChunkSize
	cache.mu.Unlock()

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)

		go func() {
			defer wg.Done()
			cache.CleanPieces()
		}()
	}

	wg.Wait()

	if got, wantMax := cache.filled.Load(), int64(2*memPieceChunkSize); got > wantMax {
		t.Fatalf("filled bytes after concurrent CleanPieces = %d, want <= %d", got, wantMax)
	}

	if got := len(cache.copyResidentPieces()); got > 2 {
		t.Fatalf("resident pieces after concurrent CleanPieces = %d, want <= 2", got)
	}
}

func TestGetActiveReaderRangesDoesNotHoldReadersLockWhileCheckingReader(t *testing.T) {
	setupStorageTest()

	stor := NewStorage(64 * 1024)
	cache := NewCache(64*1024, stor)

	blockedReader := &Reader{cache: cache}
	idleReader := &Reader{cache: cache}

	blockedReader.isUse.Store(false)
	idleReader.isUse.Store(false)

	cache.readers.items[blockedReader] = struct{}{}
	cache.readers.items[idleReader] = struct{}{}

	blockedReader.mu.Lock()

	done := make(chan struct{})
	go func() {
		defer close(done)

		_ = cache.getActiveReaderRanges()
	}()

	if !canLockCacheReaders(t, cache, 200*time.Millisecond) {
		blockedReader.mu.Unlock()
		t.Fatal("getActiveReaderRanges held readers.mu while waiting on an individual reader lock")
	}

	blockedReader.mu.Unlock()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("getActiveReaderRanges did not finish after blocked reader lock was released")
	}
}

func canLockCacheReaders(t *testing.T, cache *Cache, timeout time.Duration) bool {
	t.Helper()

	locked := make(chan struct{})

	go func() {
		cache.readers.mu.Lock()
		_ = len(cache.readers.items)
		cache.readers.mu.Unlock()
		close(locked)
	}()

	select {
	case <-locked:
		return true
	case <-time.After(timeout):
		return false
	}
}

func TestCacheMetricsSnapshotTracksAggregateCacheLifecycle(t *testing.T) {
	setupStorageTest()

	before := SnapshotCacheStats()
	stor := NewStorage(64 * 1024)
	cache := NewCache(64*1024, stor)

	info := &metainfo.Info{
		Files:       []metainfo.FileInfo{{Path: []string{"test.bin"}, Length: 32 * 1024}},
		PieceLength: 16 * 1024,
	}
	info.Pieces = make([]byte, 2*20)
	hash := metainfo.NewHashFromHex("abcdef1234567890abcdef1234567890abcdef12")
	cache.Init(info, hash)

	afterInit := SnapshotCacheStats()
	if got, want := afterInit.ActiveCaches-before.ActiveCaches, int64(1); got != want {
		t.Fatalf("active caches delta = %d, want %d", got, want)
	}

	if got, want := afterInit.ConfiguredCapacityBytes-before.ConfiguredCapacityBytes, int64(64*1024); got != want {
		t.Fatalf("configured capacity delta = %d, want %d", got, want)
	}

	if got, want := afterInit.PiecesCount-before.PiecesCount, int64(2); got != want {
		t.Fatalf("pieces count delta = %d, want %d", got, want)
	}

	cache.RecordHit()
	cache.RecordMiss()

	piece := cache.pieces[0]
	if _, err := piece.WriteAt(bytes.Repeat([]byte{0xAB}, 1024), 0); err != nil {
		t.Fatalf("WriteAt error: %v", err)
	}

	afterWrite := SnapshotCacheStats()
	if got, want := afterWrite.Hits-before.Hits, uint64(1); got != want {
		t.Fatalf("cache hits delta = %d, want %d", got, want)
	}

	if got, want := afterWrite.Misses-before.Misses, uint64(1); got != want {
		t.Fatalf("cache misses delta = %d, want %d", got, want)
	}

	if got, want := afterWrite.LogicalFilledBytes-before.LogicalFilledBytes, int64(memPieceChunkSize); got != want {
		t.Fatalf("logical filled delta = %d, want %d", got, want)
	}

	if got, want := afterWrite.InMemoryChunks-before.InMemoryChunks, int64(1); got != want {
		t.Fatalf("in-memory chunks delta = %d, want %d", got, want)
	}

	cache.SetCapacity(128 * 1024)

	afterCapacity := SnapshotCacheStats()
	if got, want := afterCapacity.ConfiguredCapacityBytes-before.ConfiguredCapacityBytes, int64(8<<20); got != want {
		t.Fatalf("configured capacity delta after SetCapacity = %d, want %d", got, want)
	}

	piece.mPiece.Release()

	afterRelease := SnapshotCacheStats()
	if got, want := afterRelease.LogicalFilledBytes-before.LogicalFilledBytes, int64(0); got != want {
		t.Fatalf("logical filled delta after Release = %d, want %d", got, want)
	}

	if got, want := afterRelease.InMemoryChunks-before.InMemoryChunks, int64(0); got != want {
		t.Fatalf("in-memory chunks delta after Release = %d, want %d", got, want)
	}

	cache.addActiveReaders(1)

	afterReader := SnapshotCacheStats()
	if got, want := afterReader.ActiveReaders-before.ActiveReaders, int64(1); got != want {
		t.Fatalf("active readers delta = %d, want %d", got, want)
	}

	if err := cache.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	afterClose := SnapshotCacheStats()
	if got, want := afterClose.ActiveCaches-before.ActiveCaches, int64(0); got != want {
		t.Fatalf("active caches delta after Close = %d, want %d", got, want)
	}

	if got, want := afterClose.ConfiguredCapacityBytes-before.ConfiguredCapacityBytes, int64(0); got != want {
		t.Fatalf("configured capacity delta after Close = %d, want %d", got, want)
	}

	if got, want := afterClose.ActiveReaders-before.ActiveReaders, int64(0); got != want {
		t.Fatalf("active readers delta after Close = %d, want %d", got, want)
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

func TestClearPriorityAsyncSchedulesSingleTimer(t *testing.T) {
	setupStorageTest()

	stor := NewStorage(1 * 1024 * 1024)
	cache := NewCache(1*1024*1024, stor)
	cache.SetTorrent(&torrent.Torrent{})

	cache.clearPriorityAsync()

	cache.priorities.clearMu.Lock()
	firstTimer := cache.priorities.clearTimer
	cache.priorities.clearMu.Unlock()

	if firstTimer == nil {
		t.Fatal("clear priority timer was not scheduled")
	}

	t.Cleanup(func() {
		firstTimer.Stop()
	})

	cache.clearPriorityAsync()

	cache.priorities.clearMu.Lock()
	secondTimer := cache.priorities.clearTimer
	cache.priorities.clearMu.Unlock()

	if secondTimer == nil {
		t.Fatal("clear priority timer was not rescheduled")
	}

	if secondTimer == firstTimer {
		t.Fatal("clear priority timer was reused instead of being replaced")
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
		{
			name:             "single reader doubles connection budget within horizon",
			connectionsLimit: 25,
			activeReaders:    1,
			pieceLength:      4 << 20,
			want:             50,
		},
		{
			name:             "two readers split and widen connection budget",
			connectionsLimit: 25,
			activeReaders:    2,
			pieceLength:      4 << 20,
			want:             24,
		},
		{
			name:             "same torrent two readers split and widen playback budget",
			connectionsLimit: 50,
			activeReaders:    2,
			pieceLength:      8 << 20,
			want:             50,
		},
		{
			name:             "priority budget has upper bound",
			connectionsLimit: 120,
			activeReaders:    1,
			pieceLength:      2 << 20,
			want:             80,
		},
		{
			name:             "three readers split connection budget",
			connectionsLimit: 6,
			activeReaders:    3,
			pieceLength:      4 << 20,
			want:             2,
		},
		{
			name:             "reader count floor",
			connectionsLimit: 4,
			activeReaders:    0,
			pieceLength:      1 << 20,
			want:             8,
		},
		{
			name:             "connection floor",
			connectionsLimit: 0,
			activeReaders:    3,
			pieceLength:      1 << 20,
			want:             1,
		},
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

func TestCachePrioritySelectionStats(t *testing.T) {
	before := SnapshotCachePriorityStats()
	cache := NewCache(64<<20, nil)

	cache.recordPrioritySelection(7, true)

	after := SnapshotCachePriorityStats()
	if got, want := after.UpdatesTotal-before.UpdatesTotal, uint64(1); got != want {
		t.Fatalf("UpdatesTotal delta = %d, want %d", got, want)
	}

	if got, want := after.DesiredPiecesTotal-before.DesiredPiecesTotal, uint64(7); got != want {
		t.Fatalf("DesiredPiecesTotal delta = %d, want %d", got, want)
	}

	if got, want := after.BudgetLimitedTotal-before.BudgetLimitedTotal, uint64(1); got != want {
		t.Fatalf("BudgetLimitedTotal delta = %d, want %d", got, want)
	}

	if after.LastUpdateUnixMS <= 0 {
		t.Fatal("LastUpdateUnixMS must be set after priority selection")
	}
}

func TestActiveReaderRanges(t *testing.T) {
	t.Parallel()

	if ranges := activeReaderRanges(nil); ranges != nil {
		t.Fatalf("activeReaderRanges(nil) = %v, want nil", ranges)
	}

	ranges := activeReaderRanges([]activeReaderSnapshot{
		{piecesRange: Range{Start: 10, End: 20}},
		{piecesRange: Range{Start: 18, End: 30}},
		{piecesRange: Range{Start: 40, End: 45}},
	})

	if got, want := len(ranges), 2; got != want {
		t.Fatalf("len(activeReaderRanges()) = %d, want %d", got, want)
	}

	if got, want := ranges[0], (Range{Start: 10, End: 30}); got != want {
		t.Fatalf("ranges[0] = %+v, want %+v", got, want)
	}

	if got, want := ranges[1], (Range{Start: 40, End: 45}); got != want {
		t.Fatalf("ranges[1] = %+v, want %+v", got, want)
	}
}

func TestCachePriorityChurnStats(t *testing.T) {
	before := SnapshotCachePriorityStats()
	cache := NewCache(64<<20, nil)

	cache.recordPriorityChurn(2, 3, 5, 8)

	after := SnapshotCachePriorityStats()
	if got, want := after.ClearedPiecesTotal-before.ClearedPiecesTotal, uint64(2); got != want {
		t.Fatalf("ClearedPiecesTotal delta = %d, want %d", got, want)
	}

	if got, want := after.SetPiecesTotal-before.SetPiecesTotal, uint64(3); got != want {
		t.Fatalf("SetPiecesTotal delta = %d, want %d", got, want)
	}

	if got, want := after.NoopPiecesTotal-before.NoopPiecesTotal, uint64(5); got != want {
		t.Fatalf("NoopPiecesTotal delta = %d, want %d", got, want)
	}

	if got, want := after.TrackedPieces-before.TrackedPieces, int64(0); got != want {
		t.Fatalf("unregistered cache tracked pieces delta = %d, want %d", got, want)
	}
}

func TestCachePriorityTrackedPiecesCleanup(t *testing.T) {
	before := SnapshotCachePriorityStats()
	cache := NewCache(64<<20, nil)
	cache.registerMetrics()

	cache.recordPriorityChurn(0, 1, 0, 6)

	afterRecord := SnapshotCachePriorityStats()
	if got, want := afterRecord.TrackedPieces-before.TrackedPieces, int64(6); got != want {
		t.Fatalf("TrackedPieces delta after record = %d, want %d", got, want)
	}

	cache.unregisterMetrics()

	afterCleanup := SnapshotCachePriorityStats()
	if got, want := afterCleanup.TrackedPieces-before.TrackedPieces, int64(0); got != want {
		t.Fatalf("TrackedPieces delta after cleanup = %d, want %d", got, want)
	}
}

func TestCacheUntrackPriorityPieceRemovesReleasedPieceState(t *testing.T) {
	before := SnapshotCachePriorityStats()
	cache := NewCache(64<<20, nil)
	cache.registerMetrics()
	t.Cleanup(cache.unregisterMetrics)

	cache.priorities.pieces[3] = torrent.PiecePriorityNow
	cache.priorities.pieces[7] = torrent.PiecePriorityReadahead
	cache.setTrackedPriorityPieces(len(cache.priorities.pieces))

	afterTrack := SnapshotCachePriorityStats()
	if got, want := afterTrack.TrackedPieces-before.TrackedPieces, int64(2); got != want {
		t.Fatalf("TrackedPieces delta after setup = %d, want %d", got, want)
	}

	if !cache.untrackPriorityPiece(3) {
		t.Fatal("untrackPriorityPiece should report tracked piece removal")
	}

	if _, ok := cache.priorities.pieces[3]; ok {
		t.Fatal("released piece should be removed from tracked priority state")
	}

	if got, want := cache.metrics.priorityTrackedPieces.Load(), int64(1); got != want {
		t.Fatalf("cache tracked priority pieces = %d, want %d", got, want)
	}

	afterUntrack := SnapshotCachePriorityStats()
	if got, want := afterUntrack.TrackedPieces-before.TrackedPieces, int64(1); got != want {
		t.Fatalf("TrackedPieces delta after untrack = %d, want %d", got, want)
	}

	if cache.untrackPriorityPiece(3) {
		t.Fatal("untrackPriorityPiece should not report removal twice")
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
		{name: "reader readahead window", pieceID: 14, readerPos: 10, readerRAH: 14, wantPrio: torrent.PiecePriorityReadahead},
		{name: "short high tail", pieceID: 19, readerPos: 10, readerRAH: 14, wantPrio: torrent.PiecePriorityHigh},
		{name: "normal after short tail", pieceID: 20, readerPos: 10, readerRAH: 14, wantPrio: torrent.PiecePriorityNormal},
		{name: "normal far tail", pieceID: 90, readerPos: 10, readerRAH: 14, wantPrio: torrent.PiecePriorityNormal},
		{name: "normal distant tail", pieceID: 120, readerPos: 10, readerRAH: 14, wantPrio: torrent.PiecePriorityNormal},
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

func TestDesiredPrioritiesForReaderAtFileStart(t *testing.T) {
	cache := NewCache(64<<20, testCacheHost{
		sets: &settings.BTSets{
			ConnectionsLimit: 80,
		},
	})
	cache.pieceLength = 4 << 20
	cache.pieces = make(map[int]*Piece)

	for i := range 10 {
		cache.pieces[i] = &Piece{ID: i}
	}

	ranges := []Range{{Start: 0, End: 9}}
	readers := []activeReaderSnapshot{
		{
			readerPos:    0,
			readerRAHPos: 4,
			piecesRange:  Range{Start: 0, End: 9},
		},
	}

	desired := cache.desiredPrioritiesForReaders(ranges, readers)
	if got, want := desired[0], torrent.PiecePriorityNow; got != want {
		t.Fatalf("desired[0] = %v, want %v", got, want)
	}

	if got, want := desired[1], torrent.PiecePriorityNext; got != want {
		t.Fatalf("desired[1] = %v, want %v", got, want)
	}

	if got, want := desired[4], torrent.PiecePriorityReadahead; got != want {
		t.Fatalf("desired[4] = %v, want %v", got, want)
	}

	if got, want := desired[5], torrent.PiecePriorityHigh; got != want {
		t.Fatalf("desired[5] = %v, want %v", got, want)
	}
}
