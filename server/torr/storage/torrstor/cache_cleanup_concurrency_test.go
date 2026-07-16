package torrstor

import (
	"bytes"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/anacrolix/torrent/metainfo"
)

func TestCleanPiecesFollowsCapacityShrinkDuringRunningPass(t *testing.T) {
	cache := newCleanupTestCache(t, 8, 4)
	beforeRuns := cache.metrics.cleanupRuns.Load()

	cache.resident.mu.Lock()
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		cache.CleanPieces()
	}()

	waitForCleanupRuns(t, cache, beforeRuns+1)
	setCleanupTestCapacity(cache, 2*memPieceChunkSize)

	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		cache.CleanPieces()
	}()

	waitForCleanupPending(t, cache)
	cache.resident.mu.Unlock()

	waitForCleanupCall(t, firstDone)
	waitForCleanupCall(t, secondDone)

	if got, wantMax := cache.filled.Load(), int64(2*memPieceChunkSize); got > wantMax {
		t.Fatalf("filled bytes after capacity shrink = %d, want <= %d", got, wantMax)
	}

	if got, want := cache.metrics.cleanupRuns.Load()-beforeRuns, uint64(2); got != want {
		t.Fatalf("cleanup passes = %d, want %d", got, want)
	}
}

func TestCleanPiecesConcurrentCallsTerminateWithoutRemovablePieces(t *testing.T) {
	cache := newCleanupTestCache(t, 8, 2)

	cache.resident.mu.Lock()
	cache.resident.items = make(map[int]*Piece)
	cache.resident.mu.Unlock()

	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)

		go func() {
			defer wg.Done()
			cache.CleanPieces()
		}()
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	waitForCleanupCall(t, done)
	assertCleanupIdle(t, cache)
}

func TestCleanPiecesCloseWakesConcurrentCallers(t *testing.T) {
	cache := newCleanupTestCache(t, 8, 2)
	beforeRuns := cache.metrics.cleanupRuns.Load()

	cache.resident.mu.Lock()
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		cache.CleanPieces()
	}()
	waitForCleanupRuns(t, cache, beforeRuns+1)

	waiterDone := make(chan struct{})
	go func() {
		defer close(waiterDone)
		cache.CleanPieces()
	}()
	waitForCleanupPending(t, cache)

	closeDone := make(chan struct{})
	go func() {
		defer close(closeDone)

		if err := cache.Close(); err != nil {
			t.Errorf("Cache.Close() error = %v", err)
		}
	}()

	waitForCacheClosed(t, cache)
	cache.resident.mu.Unlock()

	waitForCleanupCall(t, firstDone)
	waitForCleanupCall(t, waiterDone)
	waitForCleanupCall(t, closeDone)
	assertCleanupIdle(t, cache)
}

func newCleanupTestCache(t *testing.T, pieceCount int, capacityPieces int64) *Cache {
	t.Helper()
	setupStorageTest()
	drainMemPieceChunkPoolForTest(t)

	cache := NewCache(1<<20, NewStorage(1<<20))
	info := &metainfo.Info{
		Files:       []metainfo.FileInfo{{Path: []string{"test.bin"}, Length: int64(pieceCount) * memPieceChunkSize}},
		PieceLength: memPieceChunkSize,
		Pieces:      make([]byte, pieceCount*20),
	}
	cache.Init(info, metainfo.NewHashFromHex("abcdef1234567890abcdef1234567890abcdef12"))
	cache.cleanup.lastRunNano.Store(time.Now().UnixNano())

	for id := range pieceCount {
		data := bytes.Repeat([]byte{byte(id)}, memPieceChunkSize)
		if _, err := cache.pieces[id].WriteAt(data, 0); err != nil {
			t.Fatalf("WriteAt piece %d error: %v", id, err)
		}
	}

	setCleanupTestCapacity(cache, capacityPieces*memPieceChunkSize)
	t.Cleanup(func() {
		if !cache.isClosed.Load() {
			if err := cache.Close(); err != nil {
				t.Errorf("Cache.Close() error = %v", err)
			}
		}
	})

	return cache
}

func setCleanupTestCapacity(cache *Cache, capacity int64) {
	cache.mu.Lock()
	cache.capacity = capacity
	cache.mu.Unlock()
}

func waitForCleanupRuns(t *testing.T, cache *Cache, want uint64) {
	t.Helper()
	waitForCleanupCondition(t, func() bool {
		return cache.metrics.cleanupRuns.Load() >= want
	})
}

func waitForCleanupPending(t *testing.T, cache *Cache) {
	t.Helper()
	waitForCleanupCondition(t, func() bool {
		cache.cleanup.mu.Lock()
		defer cache.cleanup.mu.Unlock()

		return cache.cleanup.running && cache.cleanup.pending
	})
}

func waitForCacheClosed(t *testing.T, cache *Cache) {
	t.Helper()
	waitForCleanupCondition(t, cache.isClosed.Load)
}

func waitForCleanupCondition(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for cleanup condition")
		}

		runtime.Gosched()
	}
}

func waitForCleanupCall(t *testing.T, done <-chan struct{}) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for cleanup call")
	}
}

func assertCleanupIdle(t *testing.T, cache *Cache) {
	t.Helper()
	cache.cleanup.mu.Lock()
	defer cache.cleanup.mu.Unlock()

	if cache.cleanup.running || cache.cleanup.pending {
		t.Fatalf(
			"cleanup state running=%v pending=%v, want idle",
			cache.cleanup.running,
			cache.cleanup.pending,
		)
	}
}
