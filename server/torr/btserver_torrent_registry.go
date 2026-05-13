package torr

import (
	"maps"
	"sync"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

type btTorrentRegistry struct {
	mu    sync.RWMutex
	items map[metainfo.Hash]*Torrent
}

func newBTTorrentRegistry() *btTorrentRegistry {
	return &btTorrentRegistry{
		items: make(map[metainfo.Hash]*Torrent),
	}
}

func (r *btTorrentRegistry) Get(hash torrent.InfoHash) *Torrent {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	torr := r.items[hash]
	r.mu.RUnlock()

	return torr
}

func (r *btTorrentRegistry) List() map[metainfo.Hash]*Torrent {
	if r == nil {
		return map[metainfo.Hash]*Torrent{}
	}

	r.mu.RLock()
	list := make(map[metainfo.Hash]*Torrent, len(r.items))
	maps.Copy(list, r.items)
	r.mu.RUnlock()

	return list
}

func (r *btTorrentRegistry) Len() int {
	if r == nil {
		return 0
	}

	r.mu.RLock()
	n := len(r.items)
	r.mu.RUnlock()

	return n
}

func (r *btTorrentRegistry) Delete(hash torrent.InfoHash) *Torrent {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	torr := r.items[hash]
	delete(r.items, hash)
	r.mu.Unlock()

	return torr
}

func (r *btTorrentRegistry) LoadOrStore(hash torrent.InfoHash, torr *Torrent) (*Torrent, bool) {
	if r == nil {
		return torr, false
	}

	r.mu.Lock()
	existing, ok := r.items[hash]
	if ok {
		r.mu.Unlock()

		return existing, true
	}

	r.items[hash] = torr
	r.mu.Unlock()

	return torr, false
}

func (r *btTorrentRegistry) Snapshot() []*Torrent {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	torrents := make([]*Torrent, 0, len(r.items))
	for _, torr := range r.items {
		torrents = append(torrents, torr)
	}
	r.mu.RUnlock()

	return torrents
}

func (r *btTorrentRegistry) Reset() {
	if r == nil {
		return
	}

	r.mu.Lock()
	r.items = make(map[metainfo.Hash]*Torrent)
	r.mu.Unlock()
}
