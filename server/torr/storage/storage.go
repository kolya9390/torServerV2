package storage

import (
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

// Storage defines the interface for torrent storage operations.
type Storage interface {
	storage.ClientImpl

	// CloseHash closes storage for a specific torrent hash.
	CloseHash(hash metainfo.Hash)
}
