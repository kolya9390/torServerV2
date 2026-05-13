package torr

import (
	"time"

	"github.com/anacrolix/torrent"

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

	localReaders := t.ActiveReaders()

	playbackTorrents := estimatePlaybackTorrents(GetActiveStreams(), localReaders)
	if t.bt != nil {
		playbackTorrents = t.bt.ActivePlaybackTorrents()
	}

	if t.Torrent != nil {
		targetConns := adaptiveMaxEstablishedConns(sets.ConnectionsLimit, playbackTorrents, localReaders)
		if current := int(t.lifecycle.lastMaxEstablished); current != targetConns {
			t.SetMaxEstablishedConns(targetConns)
			t.lifecycle.lastMaxEstablished = int32(targetConns)
		}
	}

	if localReaders == 0 {
		return
	}

	adj := adaptiveReadahead(baseCap, playbackTorrents)
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

	return cache.NewReader(file)
}

func (t *Torrent) CloseReader(reader *torrstor.Reader) {
	if reader == nil || t == nil || t.cache == nil {
		return
	}

	t.cache.CloseReader(reader)
	t.AddExpiredTime(time.Second * time.Duration(t.currentSettings().TorrentDisconnectTimeout))
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
