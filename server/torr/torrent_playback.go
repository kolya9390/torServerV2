package torr

import (
	"time"

	"github.com/anacrolix/torrent"

	"server/settings"
	"server/torr/state"
	"server/torr/storage/torrstor"
)

func (t *Torrent) updateRA() {
	if t.cache == nil {
		return
	}

	sets := t.currentSettings()

	baseCap := sets.CacheSize
	if baseCap <= 0 {
		baseCap = t.cache.GetCapacity()
	}

	readerActivity := t.cache.ReaderActivitySnapshot(time.Now())
	localReaders := readerActivity.ActiveReaders

	playbackTorrents := estimatePlaybackTorrents(GetActiveStreams(), localReaders)
	if t.bt != nil {
		playbackTorrents = t.bt.ActivePlaybackTorrents()
	}

	if t.Torrent != nil {
		targetConns := adaptiveMaxEstablishedConnsForReaderAge(
			sets,
			playbackTorrents,
			localReaders,
			time.Duration(readerActivity.OldestReaderAgeMS)*time.Millisecond,
		)
		if current := int(t.lifecycle.lastMaxEstablished.Load()); current != targetConns {
			t.SetMaxEstablishedConns(targetConns)
			t.lifecycle.lastMaxEstablished.Store(int32(targetConns))
		}
	}

	if localReaders == 0 {
		return
	}

	t.maybeBoostPeerAcquisition(playbackTorrents)

	adj := adaptiveReadahead(baseCap, playbackTorrents, sets.StreamConfig())
	t.cache.AdjustRA(adj)

	if time.Since(t.lifecycle.lastPriorityUpdate) >= adaptivePriorityInterval(playbackTorrents) {
		t.lifecycle.lastPriorityUpdate = time.Now()
		t.cache.RequestPriorityUpdate()
	}
}

func (t *Torrent) NewReader(file *torrent.File) *torrstor.Reader {
	t.muTorrent.Lock()
	closed := t.Stat == state.TorrentClosed
	cache := t.cache
	t.muTorrent.Unlock()

	if closed || cache == nil {
		return nil
	}

	reader := cache.NewReader(file)
	if reader != nil {
		t.resumePlaybackPeerAcquisition()
	}

	return reader
}

// TouchPlaybackIntent keeps a torrent ready for imminent stream/preload work.
// Some clients poll stat/preload before opening a real playback reader, so the
// lifecycle must not quiesce network reads between those requests.
func (t *Torrent) TouchPlaybackIntent() {
	if t == nil {
		return
	}

	t.AddExpiredTime(disconnectTimeout(t.currentSettings()))
	t.resumePlaybackPeerAcquisition()
}

func (t *Torrent) CloseReader(reader *torrstor.Reader) {
	if reader == nil || t == nil || t.cache == nil {
		return
	}

	t.cache.CloseReader(reader)

	if t.ActiveReaders() == 0 {
		t.quiescePlaybackPeerAcquisition()
		t.ShortenExpiredTime(postPlaybackDisconnectDelay(t.currentSettings()))

		return
	}

	t.AddExpiredTime(disconnectTimeout(t.currentSettings()))
}

func (t *Torrent) GetCache() *torrstor.Cache {
	return t.cache
}

// ActiveReaders returns count of active cache readers for this torrent.
func (t *Torrent) ActiveReaders() int {
	if t == nil || t.cache == nil {
		return 0
	}

	return t.cache.GetUseReaders()
}

func (t *Torrent) resumePlaybackPeerAcquisition() {
	if t == nil || t.Torrent == nil {
		return
	}

	if t.lifecycle.playbackQuiesced.Swap(false) {
		t.AllowDataDownload()
	}

	sets := t.currentSettings()

	playbackTorrents := estimatePlaybackTorrents(GetActiveStreams(), t.ActiveReaders())
	if t.bt != nil {
		playbackTorrents = t.bt.ActivePlaybackTorrents()
	}

	targetConns := adaptiveMaxEstablishedConns(sets, playbackTorrents, t.ActiveReaders())
	t.SetMaxEstablishedConns(targetConns)
	t.lifecycle.lastMaxEstablished.Store(int32(targetConns))
	t.maybeBoostPeerAcquisition(playbackTorrents)
}

func (t *Torrent) quiescePlaybackPeerAcquisition() {
	if t == nil || t.Torrent == nil {
		return
	}

	t.DisallowDataDownload()
	t.SetMaxEstablishedConns(0)
	t.lifecycle.lastMaxEstablished.Store(0)
	t.lifecycle.playbackQuiesced.Store(true)
}

func disconnectTimeout(sets *settings.BTSets) time.Duration {
	if sets == nil || sets.TorrentDisconnectTimeout <= 0 {
		return 30 * time.Second
	}

	return time.Duration(sets.TorrentDisconnectTimeout) * time.Second
}

func postPlaybackDisconnectDelay(sets *settings.BTSets) time.Duration {
	timeout := disconnectTimeout(sets)
	if sets == nil || !isTCPOnlyBalancedCoreProfile(sets.CoreProfile) {
		return timeout
	}

	const tcpOnlyBalancedPostPlaybackIdleTimeout = 5 * time.Second
	if timeout < tcpOnlyBalancedPostPlaybackIdleTimeout {
		return timeout
	}

	return tcpOnlyBalancedPostPlaybackIdleTimeout
}
