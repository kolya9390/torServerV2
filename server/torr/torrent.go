package torr

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"server/settings"
	"server/torr/state"
	"server/torr/storage/torrstor"
	"server/torr/utils"
)

type torrentTransferState struct {
	lastSample          time.Time
	downloadSpeed       float64
	uploadSpeed         float64
	bytesReadUsefulData int64
	bytesWrittenData    int64
}

type torrentPreloadState struct {
	targetBytes int64
	loadedBytes int64
}

type torrentMediaState struct {
	durationSeconds float64
	bitRate         string
}

type torrentLifecycleState struct {
	expiredUnixNano    atomic.Int64
	closed             <-chan struct{}
	progressTicker     *time.Ticker
	lastPriorityUpdate time.Time
	lastMaxEstablished int32
}

type torrentStatusCacheState struct {
	fileIndex       map[int]*torrent.File
	cachedFileStats []*state.TorrentFileStat
	cachedTorrsHash string
}

type Torrent struct {
	Title    string
	Category string
	Poster   string
	Data     string
	*torrent.TorrentSpec

	Stat      state.TorrentStat
	Timestamp int64
	Size      int64

	*torrent.Torrent
	muTorrent sync.Mutex

	bt    *BTServer
	cache *torrstor.Cache

	transfer    torrentTransferState
	preload     torrentPreloadState
	media       torrentMediaState
	lifecycle   torrentLifecycleState
	statusCache torrentStatusCacheState
}

func trackerBudget(sets *settings.BTSets) int {
	if sets == nil {
		sets = &settings.BTSets{}
	}

	maxTrackers := 128

	// Preserve a much larger tracker pool than before. Rare 4K swarms often
	// benefit from the broader announce surface, and the original TorrServer
	// didn't aggressively trim trackers at all.
	if sets.DisableDHT && sets.DisablePEX {
		maxTrackers = 192
	}

	// Slightly adapt to connection profile.
	if sets.ConnectionsLimit > 0 {
		switch {
		case sets.ConnectionsLimit >= 80:
			maxTrackers = 192
		case sets.ConnectionsLimit <= 16:
			maxTrackers = 96
		}
	}

	return maxTrackers
}

func (t *Torrent) currentSettings() *settings.BTSets {
	if t != nil && t.bt != nil {
		if sets := t.bt.currentSettings(); sets != nil {
			return sets
		}
	}

	return &settings.BTSets{}
}

func (t *Torrent) currentRuntimeState() settings.RuntimeState {
	if t != nil && t.bt != nil {
		return t.bt.currentRuntimeState()
	}

	return settings.RuntimeState{}
}

func (t *Torrent) isReadOnly() bool {
	if t != nil && t.bt != nil && t.bt.deps.settingsProvider != nil {
		return t.bt.deps.settingsProvider.ReadOnly()
	}

	return settings.IsReadOnlyMode()
}

func (t *Torrent) ResponsiveModeEnabled() bool {
	curSets := t.currentSettings()
	if curSets == nil {
		return false
	}

	return curSets.StreamConfig().ResponsiveMode
}

func estimatePlaybackTorrents(activeStreams int32, localReaders int) int {
	totalStreams := int(activeStreams)
	if totalStreams <= 0 {
		return 1
	}

	if localReaders < 0 {
		localReaders = 0
	}

	// Readers are per torrent, while activeStreams is global.
	// Convert to an approximate count of concurrently playing torrents:
	// all global streams except local readers + this current torrent.
	torrents := totalStreams - localReaders + 1
	if torrents < 1 {
		return 1
	}

	return torrents
}

func adaptiveMinCacheCapacity(baseCap int64, playbackTorrents int) int64 {
	if baseCap <= 0 {
		return 16 << 20
	}

	if playbackTorrents <= 0 {
		playbackTorrents = 1
	}

	minCap := baseCap / 4
	if playbackTorrents > 4 {
		minCap = baseCap / int64(playbackTorrents)
	}

	if minCap < 16<<20 {
		minCap = 16 << 20
	}

	if minCap > 64<<20 {
		minCap = 64 << 20
	}

	return minCap
}

func adaptiveCacheCapacity(baseCap int64, playbackTorrents int) int64 {
	_ = playbackTorrents

	if baseCap <= 0 {
		return 0
	}

	return baseCap
}

func adaptiveReadahead(cacheCap int64, playbackTorrents int) int64 {
	_ = playbackTorrents

	// Original TorrServer keeps playback readahead simple and fixed. That
	// narrower, predictable horizon tends to produce less request/cancel churn
	// than the more adaptive V2 loop under range-heavy media clients.
	ra := int64(16 << 20)
	if cacheCap > 0 && cacheCap < ra {
		return cacheCap
	}

	return ra
}

func adaptivePriorityInterval(playbackTorrents int) time.Duration {
	_ = playbackTorrents

	return time.Second
}

func adaptiveMaxEstablishedConns(configuredLimit, playbackTorrents, localReaders int) int {
	_ = playbackTorrents
	_ = localReaders

	return effectiveEstablishedConns(configuredLimit, 50)
}

func NewTorrent(spec *torrent.TorrentSpec, bt *BTServer) (*Torrent, error) {
	// https://github.com/anacrolix/torrent/issues/747
	if bt == nil || bt.client == nil {
		return nil, errors.New("BT client not connected")
	}

	sets := bt.currentSettings()
	enableIPv6 := sets.EnableIPv6
	if bt.config != nil && bt.config.DisableIPv6 {
		enableIPv6 = false
	}

	switch sets.RetrackersMode {
	case 1:
		spec.Trackers = append(spec.Trackers, [][]string{utils.GetDefTrackers()}...)
	case 2:
		spec.Trackers = nil
	case 3:
		spec.Trackers = [][]string{utils.GetDefTrackers()}
	}

	trackers := utils.GetTrackerFromFileAtPath(bt.currentRuntimeState().PathConfig().Path)
	if len(trackers) > 0 {
		spec.Trackers = append(spec.Trackers, [][]string{trackers}...)
	}

	spec.Trackers = utils.NormalizeTrackers(spec.Trackers, enableIPv6, trackerBudget(sets))

	goTorrent, _, err := bt.client.AddTorrentSpec(spec)
	if err != nil {
		return nil, err
	}

	if tor := bt.registry.Get(spec.InfoHash); tor != nil {
		return tor, nil
	}

	timeout := min(time.Second*time.Duration(sets.TorrentDisconnectTimeout), time.Minute)

	torr := new(Torrent)
	torr.Torrent = goTorrent
	torr.Stat = state.TorrentAdded
	torr.transfer.lastSample = time.Now()
	torr.bt = bt
	torr.lifecycle.closed = goTorrent.Closed()
	torr.TorrentSpec = spec
	torr.AddExpiredTime(timeout)
	torr.Timestamp = time.Now().Unix()

	go torr.watch()

	if existing, loaded := bt.registry.LoadOrStore(spec.InfoHash, torr); loaded {
		return existing, nil
	}

	return torr, nil
}

func (t *Torrent) Files() []*torrent.File {
	if t.Torrent != nil && t.Info() != nil {
		files := t.Torrent.Files()

		return files
	}

	return nil
}

func (t *Torrent) Hash() metainfo.Hash {
	if t.Torrent != nil {
		return t.InfoHash()
	}

	if t.TorrentSpec != nil {
		return t.TorrentSpec.InfoHash
	}

	return [20]byte{}
}

func (t *Torrent) Length() int64 {
	if t.Info() == nil {
		return 0
	}

	return t.Torrent.Length()
}

// buildFileIndex constructs the file ID to *torrent.File map for O(1) lookup.
// Must be called with t.muTorrent held.
func (t *Torrent) buildFileIndex() {
	t.statusCache.fileIndex = make(map[int]*torrent.File)
	files := t.Files()

	for i, f := range files {
		t.statusCache.fileIndex[i+1] = f
	}
}

// getFileByID returns the torrent file by its 1-based ID.
// Uses cached fileIndex for O(1) lookup, building it lazily on first access.
func (t *Torrent) getFileByID(fileID int) *torrent.File {
	t.muTorrent.Lock()
	defer t.muTorrent.Unlock()

	if t.statusCache.fileIndex == nil {
		t.buildFileIndex()
	}

	return t.statusCache.fileIndex[fileID]
}
