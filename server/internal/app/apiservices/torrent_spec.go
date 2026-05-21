package apiservices

import (
	"github.com/anacrolix/torrent"

	"server/internal/app/contracts"
)

func wrapTorrentSpec(spec *torrent.TorrentSpec) contracts.TorrentSpec {
	if spec == nil {
		return contracts.TorrentSpec{}
	}

	return contracts.NewTorrentSpec(spec.InfoHash.HexString(), spec)
}

func unwrapTorrentSpec(spec contracts.TorrentSpec) (*torrent.TorrentSpec, bool) {
	raw, ok := spec.Native().(*torrent.TorrentSpec)
	if !ok || raw == nil {
		return nil, false
	}

	return raw, true
}
