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
	playbackQuiesced   atomic.Bool
	lastPeerBoostNano  atomic.Int64
	closed             <-chan struct{}
	progressTicker     *time.Ticker
	lastPriorityUpdate time.Time
	lastMaxEstablished atomic.Int32
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
	return connectionPolicyForSettings(sets, defaultEstablishedConns).trackerBudget
}

// TrackerBudgetForSettings exposes the tracker fan-out policy for diagnostics.
func TrackerBudgetForSettings(sets *settings.BTSets) int {
	return trackerBudget(sets)
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

func adaptiveCacheCapacity(baseCap int64, playbackTorrents int) int64 {
	_ = playbackTorrents

	if baseCap <= 0 {
		return 0
	}

	return baseCap
}

func adaptiveReadahead(cacheCap int64, playbackTorrents int, cfg settings.StreamConfig) int64 {
	if cacheCap <= 0 {
		return 0
	}

	minRA := readaheadBoundBytes(cfg.AdaptiveRAMinMB, 4)
	maxRA := readaheadBoundBytes(cfg.AdaptiveRAMaxMB, 64)

	if maxRA < minRA {
		maxRA = minRA
	}

	target := maxRA
	if playbackTorrents > 2 {
		target = maxRA * 2 / int64(playbackTorrents)
	}

	if target < minRA {
		target = minRA
	}

	if target > cacheCap {
		return cacheCap
	}

	return target
}

func readaheadBoundBytes(valueMB, fallbackMB int) int64 {
	if valueMB <= 0 {
		valueMB = fallbackMB
	}

	return int64(valueMB) << 20
}

func adaptivePriorityInterval(playbackTorrents int) time.Duration {
	if playbackTorrents > 1 {
		return 2 * time.Second
	}

	return time.Second
}

func adaptiveMaxEstablishedConns(sets *settings.BTSets, playbackTorrents, localReaders int) int {
	return adaptiveMaxEstablishedConnsForReaderAge(sets, playbackTorrents, localReaders, 0)
}

func adaptiveMaxEstablishedConnsForReaderAge(
	sets *settings.BTSets,
	playbackTorrents int,
	localReaders int,
	oldestReaderAge time.Duration,
) int {
	policy := connectionPolicyForSettings(sets, defaultEstablishedConns)
	target := policy.effectiveConns

	debugCfg := settings.DebugConfig{}
	if sets != nil {
		debugCfg = sets.DebugConfig()
	}

	stableCap := stablePeerCapForDebug(debugCfg, target)

	if !shouldApplyStablePeerRelief(sets, policy, stableCap, playbackTorrents, localReaders, oldestReaderAge) {
		return target
	}

	return stableCap
}

func shouldApplyStablePeerRelief(
	sets *settings.BTSets,
	policy connectionPolicy,
	stableCap int,
	playbackTorrents int,
	localReaders int,
	oldestReaderAge time.Duration,
) bool {
	if sets == nil {
		return false
	}

	if stableCap <= 0 || policy.effectiveConns <= stableCap {
		return false
	}

	if localReaders <= 0 {
		return false
	}

	if oldestReaderAge < stablePeerReliefMinAge {
		return false
	}

	return playbackTorrents >= 1
}

func stablePeerCapForDebug(debugCfg settings.DebugConfig, effectiveConns int) int {
	if !debugCfg.EnableDebug || debugCfg.StablePeerCap <= 0 || effectiveConns <= 0 {
		return 0
	}

	return min(debugCfg.StablePeerCap, effectiveConns)
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

	trackers := utils.GetTrackerFromFileAtPath(bt.currentRuntimeState().PathConfig().Path)
	applyTrackerPolicy(spec, sets, enableIPv6, trackers)

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

func applyTrackerPolicy(spec *torrent.TorrentSpec, sets *settings.BTSets, enableIPv6 bool, fileTrackers []string) {
	if spec == nil {
		return
	}

	if sets == nil {
		sets = &settings.BTSets{}
	}

	switch sets.RetrackersMode {
	case 1:
		spec.Trackers = append(spec.Trackers, [][]string{utils.GetDefTrackers()}...)
	case 2:
		spec.Trackers = nil
	case 3:
		spec.Trackers = [][]string{utils.GetDefTrackers()}
	}

	if len(fileTrackers) > 0 {
		spec.Trackers = append(spec.Trackers, [][]string{fileTrackers}...)
	}

	spec.Trackers = utils.NormalizeTrackers(spec.Trackers, enableIPv6, trackerBudget(sets))
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
