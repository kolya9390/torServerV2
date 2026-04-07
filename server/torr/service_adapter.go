package torr

import (
	"github.com/anacrolix/torrent"
)

// btserverAdapter adapts BTServer global functions to TorrentService interface.
type btserverAdapter struct{}

// NewTorrentService creates a TorrentService that uses the global BTServer.
func NewTorrentService() TorrentService {
	return &btserverAdapter{}
}

func (a *btserverAdapter) AddTorrent(spec *torrent.TorrentSpec, title, poster, data, category string) (*Torrent, error) {
	return AddTorrent(spec, title, poster, data, category)
}

func (a *btserverAdapter) GetTorrent(hash string) *Torrent {
	return GetTorrent(hash)
}

func (a *btserverAdapter) SetTorrent(hash, title, poster, category, data string) *Torrent {
	return SetTorrent(hash, title, poster, category, data)
}

func (a *btserverAdapter) RemoveTorrent(hash string) {
	RemTorrent(hash)
}

func (a *btserverAdapter) ListTorrents() []*Torrent {
	return ListTorrent()
}

func (a *btserverAdapter) DropTorrent(hash string) {
	DropTorrent(hash)
}

func (a *btserverAdapter) GetTorrentDB(hash string) *Torrent {
	return GetTorrent(hash)
}

func (a *btserverAdapter) LoadTorrent(tor *Torrent) *Torrent {
	return LoadTorrent(tor)
}
