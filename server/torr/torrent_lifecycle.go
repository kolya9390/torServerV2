package torr

import (
	"time"

	"server/log"
	"server/torr/state"
	"server/torr/storage/torrstor"
)

func (t *Torrent) WaitInfo() bool {
	if t == nil || t.Torrent == nil {
		return false
	}

	sets := t.currentSettings()
	tm := time.NewTimer(time.Minute + time.Second*time.Duration(sets.TorrentDisconnectTimeout))
	defer tm.Stop()

	select {
	case <-t.Torrent.GotInfo():
		if t.bt != nil && t.bt.storage != nil {
			if torrstor, ok := t.bt.storage.(*torrstor.Storage); ok {
				t.cache = torrstor.GetCache(t.Hash())
				if t.cache == nil {
					log.TLogln("WaitInfo: cache not found for torrent", t.Hash().HexString())

					return false
				}

				t.cache.SetTorrent(t.Torrent)
			}
		}

		return true
	case <-t.lifecycle.closed:
		return false
	case <-tm.C:
		return false
	}
}

func (t *Torrent) GotInfo() bool {
	if t == nil {
		return false
	}

	t.muTorrent.Lock()
	if t.Stat == state.TorrentClosed {
		t.muTorrent.Unlock()

		return false
	}
	if t.Stat == state.TorrentPreload {
		t.muTorrent.Unlock()

		return true
	}

	t.Stat = state.TorrentGettingInfo
	t.muTorrent.Unlock()

	if t.WaitInfo() {
		t.muTorrent.Lock()
		if t.Stat != state.TorrentPreload {
			t.Stat = state.TorrentWorking
		}
		t.muTorrent.Unlock()
		t.AddExpiredTime(time.Second * time.Duration(t.currentSettings().TorrentDisconnectTimeout))

		return true
	}

	t.Close()

	return false
}

func (t *Torrent) AddExpiredTime(duration time.Duration) {
	newExp := time.Now().Add(duration).UnixNano()

	for {
		cur := t.lifecycle.expiredUnixNano.Load()
		if cur >= newExp {
			return
		}

		if t.lifecycle.expiredUnixNano.CompareAndSwap(cur, newExp) {
			return
		}
	}
}

func (t *Torrent) watch() {
	t.lifecycle.progressTicker = time.NewTicker(time.Second)
	defer t.lifecycle.progressTicker.Stop()

	for {
		select {
		case <-t.lifecycle.progressTicker.C:
			t.progressEvent()
		case <-t.lifecycle.closed:
			return
		}
	}
}

func (t *Torrent) progressEvent() {
	if t == nil {
		return
	}

	if t.expired() {
		if t.TorrentSpec != nil {
			log.TLogln("Torrent close by timeout", t.TorrentSpec.InfoHash.HexString())
		}

		if t.bt != nil {
			t.bt.RemoveTorrent(t.Hash())
		}

		return
	}

	t.muTorrent.Lock()
	if t.Torrent != nil && t.Info() != nil {
		st := t.Stats()
		deltaDlBytes := st.BytesRead.Int64() - t.transfer.bytesReadUsefulData
		deltaUpBytes := st.BytesWritten.Int64() - t.transfer.bytesWrittenData

		deltaTime := time.Since(t.transfer.lastSample).Seconds()
		if deltaTime <= 0 {
			deltaTime = 1
		}

		t.transfer.downloadSpeed = float64(deltaDlBytes) / deltaTime
		t.transfer.uploadSpeed = float64(deltaUpBytes) / deltaTime

		t.transfer.bytesReadUsefulData = st.BytesRead.Int64()
		t.transfer.bytesWrittenData = st.BytesWritten.Int64()

		if t.cache != nil {
			t.preload.loadedBytes = t.cache.Filled()
		}
	} else {
		t.transfer.downloadSpeed = 0
		t.transfer.uploadSpeed = 0
	}
	t.muTorrent.Unlock()

	t.transfer.lastSample = time.Now()
	if t.cache != nil {
		t.updateRA()
	}
}

func (t *Torrent) expired() bool {
	if t == nil || t.cache == nil {
		return false
	}

	expNs := t.lifecycle.expiredUnixNano.Load()
	if expNs == 0 {
		return false
	}

	t.muTorrent.Lock()
	st := t.Stat
	t.muTorrent.Unlock()

	return t.cache.Readers() == 0 && expNs < time.Now().UnixNano() &&
		(st == state.TorrentWorking || st == state.TorrentClosed)
}

func shouldExpireTorrent(readers int, hasReusableReaders, recentPlayback bool, nowNs, expNs int64, st state.TorrentStat) bool {
	if readers > 0 {
		return false
	}

	if hasReusableReaders {
		return false
	}

	if recentPlayback {
		return false
	}

	if expNs == 0 || nowNs <= expNs {
		return false
	}

	return st == state.TorrentWorking || st == state.TorrentClosed
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

	if t.isReadOnly() && t.cache != nil && t.cache.GetUseReaders() > 0 {
		return false
	}

	readers := 0
	if t.cache != nil {
		readers = t.cache.Readers()
	}

	log.TLogln(
		"Torrent.Close begin",
		" hash=", t.Hash().HexString(),
		" state=", t.Stat.String(),
		" readers=", readers,
		" active_streams=", GetActiveStreams(),
	)

	t.muTorrent.Lock()
	t.Stat = state.TorrentClosed
	t.muTorrent.Unlock()

	if t.bt != nil {
		if t.bt.registry != nil {
			t.bt.registry.Delete(t.Hash())
		}

		if torrstor, ok := t.bt.storage.(*torrstor.Storage); ok {
			torrstor.CloseHash(t.Hash())
		}
	}

	t.drop()

	return true
}
