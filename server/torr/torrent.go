package torr

import (
	"errors"
	"server/torrshash"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	utils2 "server/utils"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"server/log"
	"server/settings"
	"server/torr/state"
	cacheSt "server/torr/storage/state"
	"server/torr/storage/torrstor"
	"server/torr/utils"
)

type Torrent struct {
	Title    string
	Category string
	Poster   string
	Data     string
	*torrent.TorrentSpec

	// All mutable runtime fields below are guarded by muTorrent unless explicitly atomic.
	Stat      state.TorrentStat
	Timestamp int64
	Size      int64

	*torrent.Torrent
	muTorrent sync.RWMutex

	bt    *BTServer
	cache *torrstor.Cache

	lastTimeSpeed       time.Time
	DownloadSpeed       float64
	UploadSpeed         float64
	BytesReadUsefulData int64
	BytesWrittenData    int64

	PreloadSize    int64
	PreloadedBytes int64

	DurationSeconds float64
	BitRate         string

	expiredTime time.Time

	closed <-chan struct{}

	progressTicker *time.Ticker

	closedFlag atomic.Bool

	adaptiveReadAhead atomic.Int64
}

func NewTorrent(spec *torrent.TorrentSpec, bt *BTServer) (*Torrent, error) {
	// https://github.com/anacrolix/torrent/issues/747
	if bt == nil || bt.client == nil {
		return nil, errors.New("BT client not connected")
	}
	switch settings.BTsets.RetrackersMode {
	case 1:
		spec.Trackers = append(spec.Trackers, [][]string{utils.GetDefTrackers()}...)
	case 2:
		spec.Trackers = nil
	case 3:
		spec.Trackers = [][]string{utils.GetDefTrackers()}
	}

	trackers := utils.GetTrackerFromFile()
	if len(trackers) > 0 {
		spec.Trackers = append(spec.Trackers, [][]string{trackers}...)
	}

	goTorrent, _, err := bt.client.AddTorrentSpec(spec)
	if err != nil {
		return nil, err
	}

	bt.mu.Lock()
	defer bt.mu.Unlock()
	if tor, ok := bt.torrents[spec.InfoHash]; ok {
		return tor, nil
	}

	timeout := time.Second * time.Duration(settings.BTsets.TorrentDisconnectTimeout)
	if timeout > time.Minute {
		timeout = time.Minute
	}

	torr := new(Torrent)
	torr.Torrent = goTorrent
	torr.setInitialState(state.TorrentAdded)
	torr.lastTimeSpeed = time.Now()
	torr.bt = bt
	torr.closed = goTorrent.Closed()
	torr.TorrentSpec = spec
	torr.AddExpiredTime(timeout)
	torr.Timestamp = time.Now().Unix()

	go torr.watch()

	bt.torrents[spec.InfoHash] = torr
	return torr, nil
}

func (t *Torrent) WaitInfo() bool {
	if t == nil || t.Torrent == nil {
		return false
	}

	// First check if info is already available
	if t.Torrent.Info() != nil {
		if t.bt != nil && t.bt.storage != nil {
			t.cache = t.bt.storage.GetCache(t.Hash())
			t.cache.SetTorrent(t.Torrent)
		}
		return true
	}

	// Close torrent if no info in 1 minute + TorrentDisconnectTimeout config option
	tm := time.NewTimer(time.Minute + time.Second*time.Duration(settings.BTsets.TorrentDisconnectTimeout))

	select {
	case <-t.Torrent.GotInfo():
		if t.bt != nil && t.bt.storage != nil {
			t.cache = t.bt.storage.GetCache(t.Hash())
			t.cache.SetTorrent(t.Torrent)
		}
		return true
	case <-t.closed:
		return false
	case <-tm.C:
		return false
	}
}

func (t *Torrent) GotInfo() bool {
	// log.TLogln("GotInfo state:", t.Stat)
	if t == nil {
		return false
	}
	curStat := t.currentState()
	if curStat == state.TorrentClosed {
		return false
	}
	// assume we have info in preload state
	// and dont override with TorrentWorking
	if curStat == state.TorrentPreload {
		return true
	}
	if !t.transitionState(state.TorrentGettingInfo, "got_info_request") {
		return false
	}
	if t.WaitInfo() {
		_ = t.transitionState(state.TorrentWorking, "got_info_success")
		t.AddExpiredTime(time.Second * time.Duration(settings.BTsets.TorrentDisconnectTimeout))
		return true
	} else {
		_ = t.transitionState(state.TorrentClosed, "got_info_timeout")
		t.Close()
		return false
	}
}

func (t *Torrent) AddExpiredTime(duration time.Duration) {
	newExpiredTime := time.Now().Add(duration)
	t.muTorrent.Lock()
	defer t.muTorrent.Unlock()
	if t.expiredTime.Before(newExpiredTime) {
		t.expiredTime = newExpiredTime
	}
}

func (t *Torrent) watch() {
	t.progressTicker = time.NewTicker(time.Second)
	defer t.progressTicker.Stop()

	for {
		select {
		case <-t.progressTicker.C:
			// Run inline to keep progress updates serialized for this torrent.
			t.progressEvent()
		case <-t.closed:
			return
		}
	}
}

func (t *Torrent) progressEvent() {
	if t.expired() {
		if t.TorrentSpec != nil {
			log.TLogln("Torrent close by timeout", t.TorrentSpec.InfoHash.HexString())
		}
		t.bt.RemoveTorrent(t.Hash())
		return
	}

	t.muTorrent.Lock()
	if t.Torrent != nil && t.Info() != nil {
		st := t.Stats()
		deltaDlBytes := st.BytesRead.Int64() - t.BytesReadUsefulData
		deltaUpBytes := st.BytesWritten.Int64() - t.BytesWrittenData
		deltaTime := time.Since(t.lastTimeSpeed).Seconds()

		t.DownloadSpeed = float64(deltaDlBytes) / deltaTime
		t.UploadSpeed = float64(deltaUpBytes) / deltaTime

		t.BytesReadUsefulData = st.BytesRead.Int64()
		t.BytesWrittenData = st.BytesWritten.Int64()

		if t.cache != nil {
			t.PreloadedBytes = t.cache.GetState().Filled
		}
	} else {
		t.DownloadSpeed = 0
		t.UploadSpeed = 0
	}
	t.lastTimeSpeed = time.Now()
	t.muTorrent.Unlock()
	t.updateRA()
}

func (t *Torrent) updateRA() {
	if t.cache == nil {
		return
	}
	if settings.BTsets == nil {
		return
	}
	t.muTorrent.Lock()
	if t.Torrent == nil || t.Info() == nil {
		t.muTorrent.Unlock()
		return
	}
	downloadBps := t.DownloadSpeed
	bitRate := t.BitRate
	buffered := t.PreloadedBytes
	pieceLength := t.Info().PieceLength
	t.muTorrent.Unlock()

	readers := t.cache.GetUseReaders()
	if readers == 0 {
		return
	}

	minRA := int64(settings.BTsets.AdaptiveRAMinMB) << 20
	maxRA := int64(settings.BTsets.AdaptiveRAMaxMB) << 20
	adj := computeAdaptiveReadahead(adaptiveRAInput{
		pieceLength: pieceLength,
		cacheCap:    t.cache.GetCapacity(),
		readers:     readers,
		downloadBps: downloadBps,
		bitrate:     bitRate,
		buffered:    buffered,
		currentRA:   t.cache.CurrentReadahead(),
		minRA:       minRA,
		maxRA:       maxRA,
	})
	if adj <= 0 {
		return
	}
	t.cache.AdjustRA(adj)
	t.adaptiveReadAhead.Store(adj)
}

func (t *Torrent) expired() bool {
	t.muTorrent.Lock()
	defer t.muTorrent.Unlock()
	if t.cache == nil {
		return false
	}
	// Don't expire if there are pending peers (still connecting/downloading)
	if t.Torrent != nil {
		torrentStats := t.Torrent.Stats()
		if torrentStats.PendingPeers > 0 || torrentStats.ActivePeers > 0 {
			return false
		}
	}
	// Use GetUseReaders() instead of Readers() to check if any reader is actively being used
	// Readers() counts all readers (including those with isUse=false), while GetUseReaders()
	// only counts readers where isUse=true (actively reading)
	if t.cache.GetUseReaders() > 0 {
		return false
	}
	return t.expiredTime.Before(time.Now()) && (t.Stat == state.TorrentWorking || t.Stat == state.TorrentClosed)
}

func (t *Torrent) Files() []*torrent.File {
	if t == nil || t.Torrent == nil || t.Info() == nil {
		return nil
	}
	return t.Torrent.Files()
}

func (t *Torrent) Hash() metainfo.Hash {
	if t.Torrent != nil {
		return t.Torrent.InfoHash()
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

func (t *Torrent) NewReader(file *torrent.File) *torrstor.Reader {
	t.muTorrent.Lock()
	defer t.muTorrent.Unlock()
	if t.Stat == state.TorrentClosed {
		return nil
	}
	if t.cache == nil {
		return nil
	}
	reader := t.cache.NewReader(file)
	return reader
}

func (t *Torrent) CloseReader(reader *torrstor.Reader) {
	t.muTorrent.Lock()
	cache := t.cache
	t.muTorrent.Unlock()
	if cache == nil {
		return
	}
	cache.CloseReader(reader)
	t.AddExpiredTime(time.Second * time.Duration(settings.BTsets.TorrentDisconnectTimeout))
}

func (t *Torrent) GetCache() *torrstor.Cache {
	t.muTorrent.Lock()
	defer t.muTorrent.Unlock()
	return t.cache
}

func (t *Torrent) drop() {
	t.muTorrent.Lock()
	defer t.muTorrent.Unlock()
	if t.Torrent != nil {
		t.Drop()
		t.Torrent = nil
	}
}

func (t *Torrent) Close() bool {
	if t == nil {
		return false
	}
	if settings.ReadOnly && t.cache != nil && t.cache.GetUseReaders() > 0 {
		return false
	}
	if !t.closedFlag.CompareAndSwap(false, true) {
		return false
	}

	_ = t.transitionState(state.TorrentClosed, "close")

	if t.bt != nil {
		t.bt.mu.Lock()
		delete(t.bt.torrents, t.Hash())
		t.bt.mu.Unlock()
	}

	t.drop()
	return true
}

func (t *Torrent) Status() *state.TorrentStatus {
	t.muTorrent.RLock()
	defer t.muTorrent.RUnlock()

	st := new(state.TorrentStatus)

	st.Stat = t.Stat
	st.StatString = t.Stat.String()
	st.Title = t.Title
	st.Category = t.Category
	st.Poster = t.Poster
	st.Data = t.Data
	st.Timestamp = t.Timestamp
	st.TorrentSize = t.Size
	st.BitRate = t.BitRate
	st.DurationSeconds = t.DurationSeconds
	st.AdaptiveReadAhead = t.adaptiveReadAhead.Load()

	if t.TorrentSpec != nil {
		st.Hash = t.TorrentSpec.InfoHash.HexString()
	}
	if t.Torrent != nil {
		st.Name = t.Name()
		st.Hash = t.Torrent.InfoHash().HexString()
		st.LoadedSize = t.BytesCompleted()

		st.PreloadedBytes = t.PreloadedBytes
		st.PreloadSize = t.PreloadSize
		st.DownloadSpeed = t.DownloadSpeed
		st.UploadSpeed = t.UploadSpeed

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
		st.TotalPeers = tst.TotalPeers
		st.PendingPeers = tst.PendingPeers
		st.ActivePeers = tst.ActivePeers
		st.ConnectedSeeders = tst.ConnectedSeeders
		st.HalfOpenPeers = tst.HalfOpenPeers

		if t.Info() != nil {
			st.TorrentSize = t.Length()

			files := t.Files()
			sort.Slice(files, func(i, j int) bool {
				return utils2.CompareStrings(files[i].Path(), files[j].Path())
			})
			for i, f := range files {
				st.FileStats = append(st.FileStats, &state.TorrentFileStat{
					Id:     i + 1, // in web id 0 is undefined
					Path:   f.Path(),
					Length: f.Length(),
				})
			}

			th := torrshash.New(st.Hash)
			th.AddField(torrshash.TagTitle, st.Title)
			th.AddField(torrshash.TagPoster, st.Poster)
			th.AddField(torrshash.TagCategory, st.Category)
			th.AddField(torrshash.TagSize, strconv.FormatInt(st.TorrentSize, 10))

			if t.TorrentSpec != nil {
				if len(t.Trackers) > 0 && len(t.Trackers[0]) > 0 {
					for _, tr := range t.Trackers[0] {
						th.AddField(torrshash.TagTracker, tr)
					}
				}
			}
			token, err := torrshash.Pack(th)
			if err == nil {
				st.TorrsHash = token
			}
		}
	}

	return st
}

func (t *Torrent) CacheState() *cacheSt.CacheState {
	if t.Torrent != nil && t.cache != nil {
		st := t.cache.GetState()
		st.Torrent = t.Status()
		return st
	}
	return nil
}
