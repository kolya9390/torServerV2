package torr

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	sets "server/settings"
)

func TestBTServerAdapterAddTorrentWithoutRuntime(t *testing.T) {
	t.Parallel()

	adapter := &btserverAdapter{}
	got, err := adapter.AddTorrent(nil, "", "", "", "")
	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("expected ErrRuntimeUnavailable, got %v", err)
	}

	if got != nil {
		t.Fatalf("expected nil torrent, got %#v", got)
	}
}

func TestBTServerAdapterGetTorrentWithoutRuntime(t *testing.T) {
	t.Parallel()

	adapter := &btserverAdapter{}
	if got := adapter.GetTorrent("deadbeef"); got != nil {
		t.Fatalf("expected nil torrent, got %#v", got)
	}
}

func TestCleanupTorrentDiskCacheRemoveAll(t *testing.T) {
	tmp := t.TempDir()
	curSets := &sets.BTSets{
		UseDisk:          true,
		TorrentsSavePath: tmp,
	}

	hash := "abc123"
	dir := filepath.Join(tmp, hash)
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	for _, path := range []string{
		filepath.Join(dir, "piece.bin"),
		filepath.Join(dir, "nested", "piece2.bin"),
	} {
		if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	cleanupTorrentDiskCache(hash, curSets)

	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected cache dir removed, stat err=%v", err)
	}
}

func TestCleanupTorrentDiskCacheRejectsUnsafeHash(t *testing.T) {
	tmp := t.TempDir()
	curSets := &sets.BTSets{
		UseDisk:          true,
		TorrentsSavePath: tmp,
	}

	sibling := filepath.Join(tmp, "safe-sibling")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatalf("mkdir sibling: %v", err)
	}

	unsafeHash := "../safe-sibling"
	cleanupTorrentDiskCache(unsafeHash, curSets)

	if _, err := os.Stat(sibling); err != nil {
		t.Fatalf("expected sibling directory preserved, stat err=%v", err)
	}
}

func TestBTServerAdapterRemoveTorrentCleansDiskCacheWhenTorrentNotLoaded(t *testing.T) {
	tmp := t.TempDir()
	hash := "abcdef1234567890abcdef1234567890abcdef12"
	cacheDir := filepath.Join(tmp, hash)

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(cacheDir, "piece.bin"), []byte("data"), 0o644); err != nil {
		t.Fatalf("write cache piece: %v", err)
	}

	bt := NewBTSWithProvidersRuntimeAndDB(
		btTestSettingsProvider{sets: &sets.BTSets{
			UseDisk:          true,
			TorrentsSavePath: tmp,
		}},
		sets.NewNoopArgsProvider(),
		func() sets.RuntimeState { return sets.RuntimeState{} },
		NewNoopTorrentDBStore(),
	)
	adapter := &btserverAdapter{bt: bt}

	adapter.RemoveTorrent(hash)

	if _, err := os.Stat(cacheDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected cache dir removed for unloaded torrent, stat err=%v", err)
	}
}

func TestWaitTorrentClosedReturnsWhenClosedChannelCloses(t *testing.T) {
	closed := make(chan struct{})
	torr := &Torrent{}
	torr.lifecycle.closed = closed

	done := make(chan struct{})
	go func() {
		waitTorrentClosed("abcdef", torr)
		close(done)
	}()

	close(closed)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waitTorrentClosed did not return after closed channel closed")
	}
}

func TestWaitTorrentClosedReturnsAfterTimeout(t *testing.T) {
	closed := make(chan struct{})
	torr := &Torrent{}
	torr.lifecycle.closed = closed

	start := time.Now()
	waitTorrentClosedWithTimeout("abcdef", torr, 10*time.Millisecond)

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("waitTorrentClosedWithTimeout took too long: %v", elapsed)
	}
}
