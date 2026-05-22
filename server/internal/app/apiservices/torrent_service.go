package apiservices

import (
	"time"

	"server/internal/app/contracts"
	"server/log"
	"server/torr"
	"server/torr/state"
)

func (d torrentService) Add(spec contracts.TorrentSpec, title, poster, data, category string) (contracts.TorrentHandle, error) {
	rawSpec, ok := unwrapTorrentSpec(spec)
	if !ok {
		return nil, contracts.ErrTorrentSpecInvalid
	}

	tor, err := d.backend.AddTorrent(rawSpec, title, poster, data, category)
	if err != nil {
		return nil, err
	}

	return wrapTorrent(tor), nil
}

func (d torrentService) Get(hash string) contracts.TorrentHandle {
	return wrapTorrent(d.backend.GetTorrent(hash))
}

func (torrentService) Status(tor contracts.TorrentHandle) *state.TorrentStatus {
	if tor == nil {
		return nil
	}

	return tor.Status()
}

func (d torrentService) StatusByHash(hash string) (*state.TorrentStatus, bool) {
	tor := d.backend.GetTorrent(hash)
	if tor == nil {
		return nil, false
	}

	return tor.Status(), true
}

func (d torrentService) Set(hash, title, poster, category, data string) contracts.TorrentHandle {
	return wrapTorrent(d.backend.SetTorrent(hash, title, poster, category, data))
}

func (d torrentService) SaveToDB(tor contracts.TorrentHandle) {
	raw := unwrapTorrent(tor)
	if raw == nil {
		return
	}

	if raw.Title == "" && raw.Name() != "" {
		raw.Title = raw.Name()
	}

	d.backend.SaveTorrentDB(raw)
}

func (d torrentService) Remove(hash string) {
	d.backend.RemoveTorrent(hash)
}

func (d torrentService) List() []contracts.TorrentHandle {
	list := d.backend.ListTorrents()
	if len(list) == 0 {
		return []contracts.TorrentHandle{}
	}

	handles := make([]contracts.TorrentHandle, 0, len(list))

	for _, tr := range list {
		if handle := wrapTorrent(tr); handle != nil {
			handles = append(handles, handle)
		}
	}

	return handles
}

func (d torrentService) Statuses() []*state.TorrentStatus {
	list := d.backend.ListTorrents()
	if len(list) == 0 {
		return []*state.TorrentStatus{}
	}

	stats := make([]*state.TorrentStatus, 0, len(list))

	for _, tr := range list {
		if tr == nil {
			continue
		}

		stats = append(stats, tr.Status())
	}

	return stats
}

func (d torrentService) ListHashes() []string {
	list := d.backend.ListTorrents()
	if len(list) == 0 {
		return []string{}
	}

	hashes := make([]string, 0, len(list))

	for _, tr := range list {
		if tr == nil {
			continue
		}

		hash := tr.Hash().HexString()
		if hash != "" {
			hashes = append(hashes, hash)
		}
	}

	return hashes
}

func (d torrentService) Drop(hash string) {
	d.backend.DropTorrent(hash)
}

func (torrentService) IsStored(tor contracts.TorrentHandle) bool {
	return tor != nil && tor.State() == state.TorrentInDB
}

func (d torrentService) DropReadiness(hash string) contracts.DropReadiness {
	tor := d.backend.GetTorrent(hash)

	readiness := contracts.DropReadiness{
		ActiveStreams:       torr.GetActiveStreams(),
		RecentStreamElapsed: torr.SinceLastStreamActivity(),
	}
	if tor != nil {
		readiness.ActiveReaders = tor.ActiveReaders()
	}

	if readiness.RecentStreamElapsed < 0 {
		readiness.RecentStreamElapsed = time.Duration(0)
	}

	return readiness
}

func (d torrentService) CacheStateByHash(hash string) (any, bool) {
	tor := d.backend.GetTorrent(hash)
	if tor == nil {
		return nil, false
	}

	return tor.CacheState(), true
}

func (d torrentService) EnqueuePreload(tor contracts.TorrentHandle, index int) bool {
	raw := unwrapTorrent(tor)
	if raw == nil {
		return false
	}

	if signals := d.runtimeSignals; signals != nil {
		if signals.ActivePlaybackTorrents() > 1 {
			log.TLogln("EnqueuePreload: skip under multi-playback load")

			return false
		}

		if !signals.HasRuntimeBackend() && signals.ActiveStreams() > 1 {
			log.TLogln("EnqueuePreload: skip under multi-stream load", "active_streams=", signals.ActiveStreams())

			return false
		}
	}

	go raw.PreloadWithSettings(index, nil)

	return true
}

func (d torrentService) EnqueueMetadataFinalize(tor contracts.TorrentHandle, spec *contracts.TorrentSpec, saveToDB bool) bool {
	raw := unwrapTorrent(tor)
	if raw == nil {
		return false
	}

	if spec != nil {
		if rawSpec, ok := unwrapTorrentSpec(*spec); ok {
			raw.TorrentSpec = rawSpec
		}
	}

	if saveToDB {
		d.backend.SaveTorrentDB(raw)
	}

	return true
}

func (d torrentService) LoadFromDB(tor contracts.TorrentHandle) contracts.TorrentHandle {
	return wrapTorrent(d.backend.LoadTorrent(unwrapTorrent(tor)))
}
