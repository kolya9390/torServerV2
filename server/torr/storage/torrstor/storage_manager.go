package torrstor

import (
	"context"
	"errors"

	"server/log"
	"server/settings"

	"github.com/anacrolix/torrent/metainfo"
	ts "github.com/anacrolix/torrent/storage"
)

type storageCacheManager struct {
	registry         storageCacheRegistry
	capacity         int64
	settingsProvider settings.SettingsProvider
}

func newStorageCacheManager(capacity int64, provider settings.SettingsProvider) *storageCacheManager {
	return &storageCacheManager{
		registry:         newStorageCacheRegistry(),
		capacity:         capacity,
		settingsProvider: provider,
	}
}

func (m *storageCacheManager) currentSettings() *settings.BTSets {
	if m != nil && m.settingsProvider != nil {
		return m.settingsProvider.Get()
	}

	return nil
}

func (m *storageCacheManager) OpenTorrent(ctx context.Context, info *metainfo.Info, infoHash metainfo.Hash) (ts.TorrentImpl, error) {
	_ = ctx

	ch := NewCache(m.capacity, m)
	ch.Init(info, infoHash)
	m.registry.Set(infoHash, ch)
	capacityFn := ch.requestStrategyCapacity

	implementation := ts.TorrentImpl{
		Piece: ch.Piece,
		Close: ch.Close,
		// Capacity bounds anacrolix request-order traversal to the bytes this
		// cache can retain; it is not only a memory-accounting value.
		Capacity: &capacityFn,
	}
	ch.registerRequestStrategyCapacityDiagnostics(implementation.Capacity != nil)

	return implementation, nil
}

func (m *storageCacheManager) CloseHash(hash metainfo.Hash) {
	if ch := m.registry.Delete(hash); ch != nil {
		if err := ch.Close(); err != nil {
			log.TLogln("Error close torrent storage cache:", err)
		}
	}
}

func (m *storageCacheManager) Close() error {
	var closeErr error

	for _, ch := range m.registry.Drain() {
		closeErr = errors.Join(closeErr, ch.Close())
	}

	return closeErr
}

func (m *storageCacheManager) GetCache(hash metainfo.Hash) *Cache {
	return m.registry.Get(hash)
}

func (m *storageCacheManager) unregisterCache(hash metainfo.Hash) {
	m.registry.Delete(hash)
}

func (m *storageCacheManager) cacheCount() int {
	return m.registry.Len()
}
