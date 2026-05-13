package torrstor

import (
	"sync"

	"github.com/anacrolix/torrent/metainfo"
)

type storageCacheRegistry struct {
	mu    sync.RWMutex
	items map[metainfo.Hash]*Cache
}

func newStorageCacheRegistry() storageCacheRegistry {
	return storageCacheRegistry{
		items: make(map[metainfo.Hash]*Cache),
	}
}

func (r *storageCacheRegistry) Set(hash metainfo.Hash, cache *Cache) {
	if r == nil {
		return
	}

	r.mu.Lock()
	r.items[hash] = cache
	r.mu.Unlock()
}

func (r *storageCacheRegistry) Get(hash metainfo.Hash) *Cache {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	cache := r.items[hash]
	r.mu.RUnlock()

	return cache
}

func (r *storageCacheRegistry) Delete(hash metainfo.Hash) *Cache {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	cache := r.items[hash]
	delete(r.items, hash)
	r.mu.Unlock()

	return cache
}

func (r *storageCacheRegistry) Drain() []*Cache {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	caches := make([]*Cache, 0, len(r.items))
	for hash, cache := range r.items {
		caches = append(caches, cache)
		delete(r.items, hash)
	}
	r.mu.Unlock()

	return caches
}

func (r *storageCacheRegistry) Len() int {
	if r == nil {
		return 0
	}

	r.mu.RLock()
	n := len(r.items)
	r.mu.RUnlock()

	return n
}
