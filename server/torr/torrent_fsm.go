package torr

import (
	"server/log"
	"server/torr/state"
)

var allowedStateTransitions = map[state.TorrentStat]map[state.TorrentStat]struct{}{
	state.TorrentAdded: {
		state.TorrentGettingInfo: {},
		state.TorrentClosed:      {},
	},
	state.TorrentGettingInfo: {
		state.TorrentWorking: {},
		state.TorrentClosed:  {},
	},
	state.TorrentWorking: {
		state.TorrentPreload:     {},
		state.TorrentGettingInfo: {},
		state.TorrentClosed:      {},
	},
	state.TorrentPreload: {
		state.TorrentWorking:     {},
		state.TorrentGettingInfo: {},
		state.TorrentClosed:      {},
	},
	state.TorrentInDB: {
		state.TorrentAdded:  {},
		state.TorrentClosed: {},
	},
	state.TorrentClosed: {},
}

func (t *Torrent) setInitialState(st state.TorrentStat) {
	t.muTorrent.Lock()
	t.Stat = st
	t.muTorrent.Unlock()
}

func (t *Torrent) transitionState(next state.TorrentStat, reason string) bool {
	t.muTorrent.Lock()
	defer t.muTorrent.Unlock()

	cur := t.Stat
	if cur == next {
		return true
	}
	if _, ok := allowedStateTransitions[cur][next]; !ok {
		log.TLogln("torrent fsm: denied transition",
			"from:", cur.String(),
			"to:", next.String(),
			"reason:", reason,
			"hash:", t.Hash().HexString())
		return false
	}
	t.Stat = next
	return true
}

func (t *Torrent) currentState() state.TorrentStat {
	t.muTorrent.Lock()
	defer t.muTorrent.Unlock()
	return t.Stat
}
