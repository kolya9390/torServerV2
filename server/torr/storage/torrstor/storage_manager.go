package torrstor

import (
	"context"

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

	return ts.TorrentImpl{
		Piece: ch.Piece,
		Close: ch.Close,
	}, nil
}

func (m *storageCacheManager) CloseHash(hash metainfo.Hash) {
	if ch := m.registry.Delete(hash); ch != nil {
		_ = ch.Close()
	}
}

func (m *storageCacheManager) Close() error {
	for _, ch := range m.registry.Drain() {
		_ = ch.Close()
	}

	return nil
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
