package torrfs

import (
	"sync"

	"server/torr"
)

type TorrentCatalog interface {
	GetTorrent(hash string) *torr.Torrent
	ListTorrents() []*torr.Torrent
}

type defaultTorrentCatalog struct{}

func (defaultTorrentCatalog) GetTorrent(hash string) *torr.Torrent {
	return nil
}

func (defaultTorrentCatalog) ListTorrents() []*torr.Torrent {
	return nil
}

var (
	catalogMu sync.RWMutex
	catalog   TorrentCatalog = defaultTorrentCatalog{}
)

func SetCatalog(next TorrentCatalog) {
	catalogMu.Lock()
	defer catalogMu.Unlock()

	if next == nil {
		catalog = defaultTorrentCatalog{}

		return
	}

	catalog = next
}

func getCatalog() TorrentCatalog {
	catalogMu.RLock()
	defer catalogMu.RUnlock()

	return catalog
}
