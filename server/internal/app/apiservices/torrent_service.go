package apiservices

import (
	"time"

	"server/internal/app/contracts"
	"server/log"
	sets "server/settings"
	"server/torr"
)

const (
	preloadAllowed preloadDecision = iota
	preloadSkippedActivePlayback
	preloadSkippedActiveStream
)

type preloadDecision int

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

func (torrentService) Status(tor contracts.TorrentHandle) *contracts.TorrentStatus {
	if tor == nil {
		return nil
	}

	return tor.Status()
}

func (d torrentService) StatusByHash(hash string) (*contracts.TorrentStatus, bool) {
	tor := d.backend.GetTorrent(hash)
	if tor == nil {
		return nil, false
	}

	return mapTorrentStatus(tor.Status()), true
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

func (d torrentService) Statuses() []*contracts.TorrentStatus {
	list := d.backend.ListTorrents()
	if len(list) == 0 {
		return []*contracts.TorrentStatus{}
	}

	stats := make([]*contracts.TorrentStatus, 0, len(list))

	for _, tr := range list {
		if tr == nil {
			continue
		}

		stats = append(stats, mapTorrentStatus(tr.Status()))
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
	return tor != nil && tor.State() == contracts.TorrentInDB
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

	switch decideStartupPreload(d.startupPreloadPolicy(), d.runtimeSignals) {
	case preloadSkippedActivePlayback:
		log.DebugSampled(
			"preload.skip.active-playback",
			20,
			"skip preload while playback is already active",
		)

		return true
	case preloadSkippedActiveStream:
		activeStreams := int32(0)
		if d.runtimeSignals != nil {
			activeStreams = d.runtimeSignals.ActiveStreams()
		}

		log.DebugSampled(
			"preload.skip.active-stream",
			20,
			"skip preload while stream is already active",
			"active_streams", activeStreams,
		)

		return true
	case preloadAllowed:
		go raw.PreloadWithSettings(index, nil)

		return true
	default:
		return false
	}
}

func (d torrentService) startupPreloadPolicy() string {
	if d.settingsProvider == nil {
		return sets.StartupPreloadPolicySkipActive
	}

	current := d.settingsProvider.Get()
	if current == nil {
		return sets.StartupPreloadPolicySkipActive
	}

	return current.StreamConfig().StartupPreloadPolicy
}

func decideStartupPreload(policy string, signals torr.RuntimeSignals) preloadDecision {
	if signals == nil {
		return preloadAllowed
	}

	activeThreshold := 0
	if sets.NormalizeStartupPreloadPolicy(policy) == sets.StartupPreloadPolicyLegacy {
		activeThreshold = 1
	}

	if signals.HasRuntimeBackend() {
		if signals.ActivePlaybackTorrents() > activeThreshold {
			return preloadSkippedActivePlayback
		}

		return preloadAllowed
	}

	if signals.ActiveStreams() > int32(activeThreshold) {
		return preloadSkippedActiveStream
	}

	return preloadAllowed
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
