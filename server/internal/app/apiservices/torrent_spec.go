package apiservices

import (
	"github.com/anacrolix/torrent"

	"server/internal/app/contracts"
)

type anacrolixTorrentSpecPayload struct {
	spec *torrent.TorrentSpec
}

func (anacrolixTorrentSpecPayload) TorrentSpecPayload() {}

func wrapTorrentSpec(spec *torrent.TorrentSpec) contracts.TorrentSpec {
	if spec == nil {
		return contracts.TorrentSpec{}
	}

	return contracts.NewTorrentSpec(spec.InfoHash.HexString(), anacrolixTorrentSpecPayload{spec: spec})
}

func unwrapTorrentSpec(spec contracts.TorrentSpec) (*torrent.TorrentSpec, bool) {
	payload, ok := spec.Payload().(anacrolixTorrentSpecPayload)
	if !ok || payload.spec == nil {
		return nil, false
	}

	return payload.spec, true
}
