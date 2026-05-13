package torr

import (
	"github.com/anacrolix/torrent"

	"server/torr/storage"
)

func (bt *BTServer) GetTorrent(hash torrent.InfoHash) *Torrent {
	if bt == nil || bt.registry == nil {
		return nil
	}

	return bt.registry.Get(hash)
}

func (bt *BTServer) ListTorrents() map[torrent.InfoHash]*Torrent {
	if bt == nil || bt.registry == nil {
		return map[torrent.InfoHash]*Torrent{}
	}

	return bt.registry.List()
}

func (bt *BTServer) LoadedTorrentCount() int {
	if bt == nil || bt.registry == nil {
		return 0
	}

	return bt.registry.Len()
}

func (bt *BTServer) RemoveTorrent(hash torrent.InfoHash) bool {
	if bt == nil || bt.registry == nil {
		return false
	}

	if torr := bt.registry.Delete(hash); torr != nil {
		return torr.Close()
	}

	return false
}

// ActivePlaybackTorrents returns an approximate count of currently active
// playback sessions.
func (bt *BTServer) ActivePlaybackTorrents() int {
	if bt == nil {
		return 1
	}

	count := 0
	torrents := bt.registry.Snapshot()

	for _, torr := range torrents {
		if torr == nil {
			continue
		}

		if torr.ActiveReaders() > 0 {
			count++
		}
	}

	if count < 1 {
		count = 1
	}

	return count
}

// Storage returns the storage implementation.
// This method implements the StorageProvider interface.
func (bt *BTServer) Storage() storage.Storage {
	return bt.storage
}
