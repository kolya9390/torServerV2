package torrstor

import (
	"context"
	"testing"

	"github.com/anacrolix/torrent/metainfo"
	ts "github.com/anacrolix/torrent/storage"

	"server/settings"
)

type storageManagerSettingsProvider struct {
	settings.SettingsProvider
	sets *settings.BTSets
}

func (p storageManagerSettingsProvider) Get() *settings.BTSets {
	return p.sets
}

func TestOpenTorrentProvidesIndependentLiveCapacity(t *testing.T) {
	const initialCapacity = int64(64 << 20)

	storage := NewStorageWithProvider(initialCapacity, settings.NewNoopSettingsProvider())

	t.Cleanup(func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	info := storageManagerTestInfo()
	firstHash := metainfo.NewHashFromHex("1111111111111111111111111111111111111111")
	secondHash := metainfo.NewHashFromHex("2222222222222222222222222222222222222222")

	first := openStorageTorrentForTest(t, storage, info, firstHash)
	second := openStorageTorrentForTest(t, storage, info, secondHash)

	if first.Capacity == nil || second.Capacity == nil {
		t.Fatal("OpenTorrent() returned an uncapped storage implementation")
	}

	if first.Capacity == second.Capacity {
		t.Fatal("independent torrent caches share one capacity pointer")
	}

	assertRequestStrategyCapacity(t, *first.Capacity, initialCapacity)
	assertRequestStrategyCapacity(t, *second.Capacity, initialCapacity)

	firstCache := storage.GetCache(firstHash)
	if firstCache == nil {
		t.Fatal("first cache was not registered")
	}

	firstCache.SetCapacity(32 << 20)

	assertRequestStrategyCapacity(t, *first.Capacity, 32<<20)
	assertRequestStrategyCapacity(t, *second.Capacity, initialCapacity)
}

func TestOpenTorrentCapacityPreservesPieceAndCloseBehavior(t *testing.T) {
	storage := NewStorageWithProvider(64<<20, settings.NewNoopSettingsProvider())
	info := storageManagerTestInfo()
	hash := metainfo.NewHashFromHex("3333333333333333333333333333333333333333")
	implementation := openStorageTorrentForTest(t, storage, info, hash)
	cache := storage.GetCache(hash)

	if cache == nil {
		t.Fatal("cache was not registered")
	}

	if got := implementation.Piece(info.Piece(0)); got != cache.pieces[0] {
		t.Fatal("TorrentImpl.Piece no longer delegates to its cache")
	}

	if err := implementation.Close(); err != nil {
		t.Fatalf("TorrentImpl.Close() error = %v", err)
	}

	if storage.GetCache(hash) != nil {
		t.Fatal("TorrentImpl.Close() did not unregister its cache")
	}

	assertRequestStrategyCapacity(t, *implementation.Capacity, 0)
}

func TestOpenTorrentRegistersDebugCapacityDiagnostics(t *testing.T) {
	provider := storageManagerSettingsProvider{
		SettingsProvider: settings.NewNoopSettingsProvider(),
		sets:             &settings.BTSets{EnableDebug: true},
	}
	storage := NewStorageWithProvider(64<<20, provider)

	t.Cleanup(func() {
		if err := storage.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	info := storageManagerTestInfo()
	hash := metainfo.NewHashFromHex("4444444444444444444444444444444444444444")
	implementation := openStorageTorrentForTest(t, storage, info, hash)

	if implementation.Capacity == nil {
		t.Fatal("OpenTorrent() returned an uncapped storage implementation")
	}

	diagnostics := SnapshotRequestStrategyCapacityDiagnostics()
	item := onlyRequestStrategyCapacityItem(t, diagnostics)

	if diagnostics.Status != "ok" || !item.ContractEnabled || !item.CapacityCapped {
		t.Fatalf("capacity diagnostics = %+v, want active bounded contract", diagnostics)
	}

	if got, want := item.BoundedRequestablePieceEstimate, int64(1); got != want {
		t.Fatalf("bounded requestable pieces = %d, want %d", got, want)
	}
}

func openStorageTorrentForTest(
	t *testing.T,
	storage *Storage,
	info *metainfo.Info,
	hash metainfo.Hash,
) ts.TorrentImpl {
	t.Helper()

	implementation, err := storage.OpenTorrent(context.Background(), info, hash)
	if err != nil {
		t.Fatalf("OpenTorrent() error = %v", err)
	}

	return implementation
}

func storageManagerTestInfo() *metainfo.Info {
	const pieceLength = int64(16 << 10)

	return &metainfo.Info{
		Name:        "storage-manager-test",
		PieceLength: pieceLength,
		Pieces:      make([]byte, 20),
		Length:      pieceLength,
	}
}
