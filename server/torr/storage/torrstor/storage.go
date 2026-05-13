package torrstor

import (
	"context"

	"server/settings"
	"server/torr/storage"

	"github.com/anacrolix/torrent/metainfo"
	ts "github.com/anacrolix/torrent/storage"
)

type Storage struct {
	storage.Storage

	manager *storageCacheManager
}

func NewStorage(capacity int64) *Storage {
	return NewStorageWithProvider(capacity, settings.DefaultSettingsProvider)
}

func NewStorageWithProvider(capacity int64, provider settings.SettingsProvider) *Storage {
	if provider == nil {
		provider = settings.NewNoopSettingsProvider()
	}

	stor := &Storage{
		manager: newStorageCacheManager(capacity, provider),
	}

	return stor
}

func (s *Storage) currentSettings() *settings.BTSets {
	if s == nil || s.manager == nil {
		return nil
	}

	return s.manager.currentSettings()
}

func (s *Storage) OpenTorrent(ctx context.Context, info *metainfo.Info, infoHash metainfo.Hash) (ts.TorrentImpl, error) {
	return s.manager.OpenTorrent(ctx, info, infoHash)
}

func (s *Storage) CloseHash(hash metainfo.Hash) {
	if s != nil && s.manager != nil {
		s.manager.CloseHash(hash)
	}
}

func (s *Storage) Close() error {
	if s == nil || s.manager == nil {
		return nil
	}

	return s.manager.Close()
}

func (s *Storage) GetCache(hash metainfo.Hash) *Cache {
	if s == nil || s.manager == nil {
		return nil
	}

	return s.manager.GetCache(hash)
}

func (s *Storage) unregisterCache(hash metainfo.Hash) {
	if s != nil && s.manager != nil {
		s.manager.unregisterCache(hash)
	}
}

func (s *Storage) cacheCount() int {
	if s == nil || s.manager == nil {
		return 0
	}

	return s.manager.cacheCount()
}
