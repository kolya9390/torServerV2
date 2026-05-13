package torr

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

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
