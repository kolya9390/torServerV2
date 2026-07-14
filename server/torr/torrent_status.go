package torr

import (
	"sort"
	"strconv"
	"time"

	"server/torr/state"
	cacheSt "server/torr/storage/state"
	"server/torr/storage/torrstor"
	"server/torrshash"
	utils2 "server/utils"

	"github.com/anacrolix/torrent"
)

// TorrentRuntimeMetrics is a lightweight, privacy-safe runtime view for diagnostics.
type TorrentRuntimeMetrics struct {
	RuntimeID        uint64 `json:"torrent_id"`
	ActivePeers      int    `json:"active_peers"`
	TotalPeers       int    `json:"total_peers"`
	PendingPeers     int    `json:"pending_peers"`
	HalfOpenPeers    int    `json:"half_open_peers"`
	ConnectedSeeders int    `json:"connected_seeders"`
	MaxEstablished   int    `json:"max_established_conns"`
	ActiveReaders    int    `json:"active_readers"`
	TotalReaders     int    `json:"total_readers"`
	IdleReaders      int    `json:"idle_readers"`
	OldestReaderMS   int64  `json:"oldest_reader_age_ms"`
	NewestReaderMS   int64  `json:"newest_reader_age_ms"`
	MaxReaderIdleMS  int64  `json:"max_reader_idle_ms"`
	PreloadActive    bool   `json:"preload_active"`
	PreloadedBytes   int64  `json:"preloaded_bytes"`
	PreloadTarget    int64  `json:"preload_target_bytes"`
	TrackerTiers     int    `json:"tracker_tiers"`
	Trackers         int    `json:"trackers"`
	DownloadSpeed    int64  `json:"download_speed"`
	UploadSpeed      int64  `json:"upload_speed"`
}

func (t *Torrent) Status() *state.TorrentStatus {
	t.muTorrent.Lock()
	defer t.muTorrent.Unlock()

	st := new(state.TorrentStatus)
	t.fillBasicStatus(st)

	if t.TorrentSpec != nil {
		st.Hash = t.TorrentSpec.InfoHash.HexString()
	}

	if t.Torrent == nil {
		return st
	}

	t.fillTorrentStatus(st)
	t.collectPeerStats(st)
	t.ensureCachedStatusDataLocked(st)
	t.collectFileStats(st)
	st.TorrsHash = t.statusCache.cachedTorrsHash

	return st
}

// fillBasicStatus populates status fields that don't require torrent info.
func (t *Torrent) fillBasicStatus(st *state.TorrentStatus) {
	st.Stat = t.Stat
	st.StatString = t.Stat.String()
	st.Title = t.Title
	st.Category = t.Category
	st.Poster = t.Poster
	st.Data = t.Data
	st.Timestamp = t.Timestamp
	st.TorrentSize = t.Size
	st.BitRate = t.media.bitRate
	st.DurationSeconds = t.media.durationSeconds
}

// fillTorrentStatus populates status fields that require a live torrent object.
func (t *Torrent) fillTorrentStatus(st *state.TorrentStatus) {
	st.Name = t.Name()
	st.Hash = t.Torrent.InfoHash().HexString()
	st.LoadedSize = t.BytesCompleted()

	st.PreloadedBytes = t.preload.loadedBytes
	st.PreloadSize = t.preload.targetBytes
	st.DownloadSpeed = t.transfer.downloadSpeed
	st.UploadSpeed = t.transfer.uploadSpeed

	tst := t.Stats()
	st.BytesWritten = tst.BytesWritten.Int64()
	st.BytesWrittenData = tst.BytesWrittenData.Int64()
	st.BytesRead = tst.BytesRead.Int64()
	st.BytesReadData = tst.BytesReadData.Int64()
	st.BytesReadUsefulData = tst.BytesReadUsefulData.Int64()
	st.ChunksWritten = tst.ChunksWritten.Int64()
	st.ChunksRead = tst.ChunksRead.Int64()
	st.ChunksReadUseful = tst.ChunksReadUseful.Int64()
	st.ChunksReadWasted = tst.ChunksReadWasted.Int64()
	st.PiecesDirtiedGood = tst.PiecesDirtiedGood.Int64()
	st.PiecesDirtiedBad = tst.PiecesDirtiedBad.Int64()
}

// collectPeerStats gathers peer-related statistics from the torrent.
func (t *Torrent) collectPeerStats(st *state.TorrentStatus) {
	tst := t.Stats()
	st.TotalPeers = tst.TotalPeers
	st.PendingPeers = tst.PendingPeers
	st.ActivePeers = tst.ActivePeers
	st.ConnectedSeeders = tst.ConnectedSeeders
	st.HalfOpenPeers = tst.HalfOpenPeers
}

// collectFileStats builds the file statistics list from torrent files.
func (t *Torrent) collectFileStats(st *state.TorrentStatus) {
	st.FileStats = t.statusCache.cachedFileStats
}

func (t *Torrent) ensureCachedStatusDataLocked(st *state.TorrentStatus) {
	if t.Info() == nil {
		return
	}

	if t.statusCache.cachedFileStats == nil {
		st.TorrentSize = t.Torrent.Length()
		files := t.Files()
		sort.Slice(files, func(i, j int) bool {
			return utils2.CompareStrings(files[i].Path(), files[j].Path())
		})

		t.statusCache.cachedFileStats = make([]*state.TorrentFileStat, 0, len(files))
		for i, f := range files {
			t.statusCache.cachedFileStats = append(t.statusCache.cachedFileStats, &state.TorrentFileStat{
				ID:     i + 1,
				Path:   f.Path(),
				Length: f.Length(),
			})
		}
	} else {
		st.TorrentSize = t.Torrent.Length()
	}

	if t.statusCache.cachedTorrsHash == "" && t.TorrentSpec != nil {
		th := torrshash.New(st.Hash)
		th.AddField(torrshash.TagTitle, st.Title)
		th.AddField(torrshash.TagPoster, st.Poster)
		th.AddField(torrshash.TagCategory, st.Category)
		th.AddField(torrshash.TagSize, strconv.FormatInt(st.TorrentSize, 10))

		if len(t.Trackers) > 0 && len(t.Trackers[0]) > 0 {
			for _, tr := range t.Trackers[0] {
				th.AddField(torrshash.TagTracker, tr)
			}
		}

		token, err := torrshash.Pack(th)
		if err == nil {
			t.statusCache.cachedTorrsHash = token
		}
	}
}

// RuntimeSnapshot returns lightweight metrics needed by periodic collectors.
func (t *Torrent) RuntimeSnapshot() (activePeers int, downloadSpeed int64, uploadSpeed int64, ok bool) {
	snapshot, ok := t.RuntimeMetricsSnapshot()
	if !ok {
		return 0, 0, 0, false
	}

	return snapshot.ActivePeers, snapshot.DownloadSpeed, snapshot.UploadSpeed, true
}

// RuntimeMetricsSnapshot returns lightweight metrics needed by periodic collectors.
func (t *Torrent) RuntimeMetricsSnapshot() (TorrentRuntimeMetrics, bool) {
	if t == nil {
		return TorrentRuntimeMetrics{}, false
	}

	t.muTorrent.Lock()
	defer t.muTorrent.Unlock()

	if t.Torrent == nil || t.Info() == nil {
		return TorrentRuntimeMetrics{}, false
	}

	stats := t.Stats()
	trackerTiers, trackers := countTrackerTiers(t.TorrentSpec)

	readerActivity := torrstor.ReaderActivitySnapshot{}
	if t.cache != nil {
		readerActivity = t.cache.ReaderActivitySnapshot(time.Now())
	}

	return TorrentRuntimeMetrics{
		RuntimeID:        t.RuntimeDiagnosticID(),
		ActivePeers:      stats.ActivePeers,
		TotalPeers:       stats.TotalPeers,
		PendingPeers:     stats.PendingPeers,
		HalfOpenPeers:    stats.HalfOpenPeers,
		ConnectedSeeders: stats.ConnectedSeeders,
		MaxEstablished:   int(t.lifecycle.lastMaxEstablished.Load()),
		ActiveReaders:    readerActivity.ActiveReaders,
		TotalReaders:     readerActivity.TotalReaders,
		IdleReaders:      readerActivity.IdleReaders,
		OldestReaderMS:   readerActivity.OldestReaderAgeMS,
		NewestReaderMS:   readerActivity.NewestReaderAgeMS,
		MaxReaderIdleMS:  readerActivity.MaxReaderIdleMS,
		PreloadActive:    t.Stat == state.TorrentPreload,
		PreloadedBytes:   t.preload.loadedBytes,
		PreloadTarget:    t.preload.targetBytes,
		TrackerTiers:     trackerTiers,
		Trackers:         trackers,
		DownloadSpeed:    int64(t.transfer.downloadSpeed),
		UploadSpeed:      int64(t.transfer.uploadSpeed),
	}, true
}

func countTrackerTiers(spec *torrent.TorrentSpec) (tiers int, trackers int) {
	if spec == nil {
		return 0, 0
	}

	for _, tier := range spec.Trackers {
		if len(tier) == 0 {
			continue
		}

		tiers++
		trackers += len(tier)
	}

	return tiers, trackers
}

func (t *Torrent) CacheState() *cacheSt.CacheState {
	if t.Torrent != nil && t.cache != nil {
		st := t.cache.GetState()
		st.Torrent = t.Status()

		return st
	}

	return nil
}
