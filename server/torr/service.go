package torr

import (
	"github.com/anacrolix/torrent"

	"server/torr/storage"
)

// TorrentService defines the interface for torrent management operations.
// This abstraction allows dependency injection and easier testing.
type TorrentService interface {
	// AddTorrent creates and starts a new torrent.
	AddTorrent(spec *torrent.TorrentSpec, title, poster, data, category string) (*Torrent, error)

	// GetTorrent returns a torrent by hash, or nil if not found.
	GetTorrent(hash string) *Torrent

	// SetTorrent updates torrent metadata.
	SetTorrent(hash, title, poster, category, data string) *Torrent

	// RemoveTorrent removes a torrent from the server.
	RemoveTorrent(hash string)

	// ListTorrents returns all active torrents.
	ListTorrents() []*Torrent

	// DropTorrent closes a torrent without saving to DB.
	DropTorrent(hash string)

	// GetTorrentDB returns torrent metadata from database.
	GetTorrentDB(hash string) *Torrent

	// LoadTorrent loads a torrent from database and starts it.
	LoadTorrent(tor *Torrent) *Torrent
}

// StorageProvider defines the interface for providing storage to the torrent client.
type StorageProvider interface {
	// Storage returns the storage implementation.
	Storage() storage.Storage
}
