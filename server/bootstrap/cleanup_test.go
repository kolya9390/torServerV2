package bootstrap

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"server/settings"
)

func TestRunCacheCleanupNoopWhenDiskCacheDisabled(t *testing.T) {
	tmp := t.TempDir()
	deps := cacheCleanupDeps{
		settingsProvider: staticSettingsProvider{cfg: &settings.BTSets{
			UseDisk:          false,
			TorrentsSavePath: tmp,
		}},
		readDir: os.ReadDir,
		remove:  os.Remove,
		listTorrents: func() []*settings.TorrentDB {
			t.Fatal("list torrent should not be called")

			return nil
		},
	}

	runCacheCleanupWithDeps(context.Background(), deps)
}

func TestRunCacheCleanupRemovesStaleHashDir(t *testing.T) {
	tmp := t.TempDir()

	staleDir := filepath.Join(tmp, "0123456789abcdef0123456789abcdef01234567")
	if err := os.Mkdir(staleDir, 0o755); err != nil {
		t.Fatalf("mkdir stale dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(staleDir, "piece.bin"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	deps := cacheCleanupDeps{
		settingsProvider: staticSettingsProvider{cfg: &settings.BTSets{
			UseDisk:           true,
			TorrentsSavePath:  tmp,
			RemoveCacheOnDrop: false,
		}},
		readDir:      os.ReadDir,
		remove:       os.Remove,
		listTorrents: func() []*settings.TorrentDB { return nil },
	}

	runCacheCleanupWithDeps(context.Background(), deps)

	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Fatalf("expected stale dir removed, stat err=%v", err)
	}
}

func TestRunCacheCleanupKeepsActiveHashDir(t *testing.T) {
	tmp := t.TempDir()
	activeHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	activeDir := filepath.Join(tmp, activeHash)
	if err := os.Mkdir(activeDir, 0o755); err != nil {
		t.Fatalf("mkdir active dir: %v", err)
	}

	deps := cacheCleanupDeps{
		settingsProvider: staticSettingsProvider{cfg: &settings.BTSets{
			UseDisk:           true,
			TorrentsSavePath:  tmp,
			RemoveCacheOnDrop: false,
		}},
		readDir: os.ReadDir,
		remove:  os.Remove,
		listTorrents: func() []*settings.TorrentDB {
			return []*settings.TorrentDB{
				{
					TorrentSpec: &torrent.TorrentSpec{
						AddTorrentOpts: torrent.AddTorrentOpts{
							InfoHash: mustHashFromHex(t, activeHash),
						},
					},
				},
			}
		},
	}

	runCacheCleanupWithDeps(context.Background(), deps)

	if _, err := os.Stat(activeDir); err != nil {
		t.Fatalf("expected active dir kept, stat err=%v", err)
	}
}

func TestRunCacheCleanupRespectsContextCancellation(t *testing.T) {
	tmp := t.TempDir()
	dirOne := filepath.Join(tmp, "1111111111111111111111111111111111111111")
	dirTwo := filepath.Join(tmp, "2222222222222222222222222222222222222222")

	if err := os.Mkdir(dirOne, 0o755); err != nil {
		t.Fatalf("mkdir dirOne: %v", err)
	}

	if err := os.Mkdir(dirTwo, 0o755); err != nil {
		t.Fatalf("mkdir dirTwo: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dirOne, "piece"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write dirOne file: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dirTwo, "piece"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write dirTwo file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	deps := cacheCleanupDeps{
		settingsProvider: staticSettingsProvider{cfg: &settings.BTSets{
			UseDisk:           true,
			TorrentsSavePath:  tmp,
			RemoveCacheOnDrop: true,
		}},
		readDir:      os.ReadDir,
		listTorrents: func() []*settings.TorrentDB { return nil },
		remove: func(path string) error {
			if path == filepath.Join(dirOne, "piece") {
				cancel()
			}

			return os.Remove(path)
		},
	}

	runCacheCleanupWithDeps(ctx, deps)

	if _, err := os.Stat(dirOne); !os.IsNotExist(err) {
		t.Fatalf("expected first dir removed, stat err=%v", err)
	}

	if _, err := os.Stat(dirTwo); err != nil {
		t.Fatalf("expected second dir to remain due cancellation, stat err=%v", err)
	}
}

func TestRemoveAllFilesRemovesNestedDirectories(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")

	nested := filepath.Join(root, "sub", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	if err := os.WriteFile(filepath.Join(nested, "piece"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write nested file: %v", err)
	}

	deps := cacheCleanupDeps{
		readDir: os.ReadDir,
		remove:  os.Remove,
	}

	if err := removeAllFiles(context.Background(), root, deps); err != nil {
		t.Fatalf("removeAllFiles returned error: %v", err)
	}

	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("expected root removed, stat err=%v", err)
	}
}

func TestRemoveAllFilesReturnsContextError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "piece"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	deps := cacheCleanupDeps{
		readDir: os.ReadDir,
		remove:  os.Remove,
	}

	err := removeAllFiles(ctx, root, deps)
	if err == nil {
		t.Fatal("expected context canceled error")
	}
}

func mustHashFromHex(t *testing.T, s string) metainfo.Hash {
	t.Helper()

	raw, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("decode hash %q: %v", s, err)
	}

	if len(raw) != 20 {
		t.Fatalf("hash length must be 20, got %d", len(raw))
	}

	var h metainfo.Hash

	copy(h[:], raw)

	return h
}

func TestRunCacheCleanupHandlesReadDirError(t *testing.T) {
	deps := cacheCleanupDeps{
		settingsProvider: staticSettingsProvider{cfg: &settings.BTSets{
			UseDisk:           true,
			TorrentsSavePath:  "/non-existent-dir",
			RemoveCacheOnDrop: true,
		}},
		readDir:      func(_ string) ([]os.DirEntry, error) { return nil, os.ErrPermission },
		remove:       os.Remove,
		listTorrents: func() []*settings.TorrentDB { return nil },
	}

	// Should not panic and should just return.
	runCacheCleanupWithDeps(context.Background(), deps)
}

type staticSettingsProvider struct {
	cfg *settings.BTSets
}

func (p staticSettingsProvider) Get() *settings.BTSets {
	return p.cfg
}

func (staticSettingsProvider) Set(*settings.BTSets) {}

func (staticSettingsProvider) ReadOnly() bool {
	return true
}

func (staticSettingsProvider) GetStaticConfig() settings.StaticConfig {
	return settings.StaticConfig{}
}

func (staticSettingsProvider) GetStoragePreferences() map[string]any {
	return map[string]any{}
}

func TestRunCacheCleanupSkipsNonHashEntries(t *testing.T) {
	tmp := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmp, "short"), 0o755); err != nil {
		t.Fatalf("mkdir short: %v", err)
	}

	if err := os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	deps := cacheCleanupDeps{
		settingsProvider: staticSettingsProvider{cfg: &settings.BTSets{
			UseDisk:           true,
			TorrentsSavePath:  tmp,
			RemoveCacheOnDrop: true,
		}},
		readDir:      os.ReadDir,
		remove:       os.Remove,
		listTorrents: func() []*settings.TorrentDB { return nil },
	}

	runCacheCleanupWithDeps(context.Background(), deps)

	if _, err := os.Stat(filepath.Join(tmp, "short")); err != nil {
		t.Fatalf("expected short dir untouched, stat err=%v", err)
	}

	if _, err := os.Stat(filepath.Join(tmp, "file.txt")); err != nil {
		t.Fatalf("expected file untouched, stat err=%v", err)
	}
}

func TestRemoveAllFilesNotExistIsNoop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	deps := cacheCleanupDeps{
		readDir: os.ReadDir,
		remove:  os.Remove,
	}

	err := removeAllFiles(ctx, filepath.Join(t.TempDir(), "missing"), deps)
	if err != nil {
		t.Fatalf("expected nil error for missing path, got %v", err)
	}
}
