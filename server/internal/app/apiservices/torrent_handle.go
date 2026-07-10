package apiservices

import (
	"net/http"

	"server/internal/app/contracts"
	"server/torr"
)

type torrentHandle struct {
	tor *torr.Torrent
}

func wrapTorrent(tor *torr.Torrent) contracts.TorrentHandle {
	if tor == nil {
		return nil
	}

	return torrentHandle{tor: tor}
}

func unwrapTorrent(handle contracts.TorrentHandle) *torr.Torrent {
	if handle == nil {
		return nil
	}

	switch h := handle.(type) {
	case torrentHandle:
		return h.tor
	case *torrentHandle:
		return h.tor
	default:
		return nil
	}
}

func (h torrentHandle) Status() *contracts.TorrentStatus {
	if h.tor == nil {
		return nil
	}

	return mapTorrentStatus(h.tor.Status())
}

func (h torrentHandle) State() contracts.TorrentState {
	if h.tor == nil {
		return contracts.TorrentClosed
	}

	return mapTorrentState(h.tor.Stat)
}

func (h torrentHandle) HashHex() string {
	if h.tor == nil {
		return ""
	}

	return h.tor.Hash().HexString()
}

func (h torrentHandle) Name() string {
	if h.tor == nil {
		return ""
	}

	return h.tor.Name()
}

func (h torrentHandle) FileCount() int {
	if h.tor == nil {
		return 0
	}

	return len(h.tor.Files())
}

func (h torrentHandle) Ready() bool {
	return h.tor != nil && h.tor.GotInfo()
}

func (h torrentHandle) EnsureTitleFromInfo() {
	if h.tor != nil {
		h.tor.EnsureTitleFromInfo()
	}
}

func (h torrentHandle) Metadata() contracts.StreamMeta {
	if h.tor == nil {
		return contracts.StreamMeta{}
	}

	meta := h.tor.Metadata()

	return contracts.StreamMeta{
		Title:    meta.Title,
		Poster:   meta.Poster,
		Category: meta.Category,
		Data:     meta.Data,
	}
}

func (h torrentHandle) TouchPlaybackIntent() {
	if h.tor != nil {
		h.tor.TouchPlaybackIntent()
	}
}

func (h torrentHandle) Stream(index int, request *http.Request, writer http.ResponseWriter) error {
	if h.tor == nil {
		return contracts.ErrPlayTorrentNotFound
	}

	return h.tor.Stream(index, request, writer)
}
