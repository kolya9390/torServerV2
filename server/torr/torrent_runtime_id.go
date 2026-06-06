package torr

import (
	"sync"
	"sync/atomic"

	"github.com/anacrolix/torrent/metainfo"
)

var runtimeTorrentIDs = torrentRuntimeIDRegistry{
	ids: make(map[metainfo.Hash]uint64),
}

type torrentRuntimeIDRegistry struct {
	mu   sync.Mutex
	next atomic.Uint64
	ids  map[metainfo.Hash]uint64
}

func torrentRuntimeID(hash metainfo.Hash) uint64 {
	runtimeTorrentIDs.mu.Lock()
	defer runtimeTorrentIDs.mu.Unlock()

	if id, ok := runtimeTorrentIDs.ids[hash]; ok {
		return id
	}

	id := runtimeTorrentIDs.next.Add(1)
	runtimeTorrentIDs.ids[hash] = id

	return id
}

// RuntimeDiagnosticID returns a process-local, non-persistent torrent id for debug metrics.
func (t *Torrent) RuntimeDiagnosticID() uint64 {
	if t == nil {
		return 0
	}

	return torrentRuntimeID(t.Hash())
}

func resetTorrentRuntimeIDsForTest() {
	runtimeTorrentIDs.mu.Lock()
	defer runtimeTorrentIDs.mu.Unlock()

	runtimeTorrentIDs.ids = make(map[metainfo.Hash]uint64)
	runtimeTorrentIDs.next.Store(0)
}
