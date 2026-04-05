package settings

import (
	"encoding/json"
	"sort"
	"sync"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

type TorrentDB struct {
	*torrent.TorrentSpec

	Title    string `json:"title,omitempty"`
	Category string `json:"category,omitempty"`
	Poster   string `json:"poster,omitempty"`
	Data     string `json:"data,omitempty"`

	Timestamp int64 `json:"timestamp,omitempty"`
	Size      int64 `json:"size,omitempty"`
}

type File struct {
	Name string `json:"name,omitempty"`
	Id   int    `json:"id,omitempty"`
	Size int64  `json:"size,omitempty"`
}

var mu sync.Mutex

func AddTorrent(torr *TorrentDB) {
	mu.Lock()
	defer mu.Unlock()

	buf, err := json.Marshal(torr)
	if err == nil {
		tdb.Set("Torrents", torr.InfoHash.HexString(), buf)
	}
}

func ListTorrent() []*TorrentDB {
	// Use read lock to prevent migration during read
	dbMigrationLock.RLock()
	defer dbMigrationLock.RUnlock()

	mu.Lock()
	defer mu.Unlock()

	var list []*TorrentDB
	keys := tdb.List("Torrents")
	for _, key := range keys {
		buf := tdb.Get("Torrents", key)
		if len(buf) > 0 {
			var torr *TorrentDB
			err := json.Unmarshal(buf, &torr)
			if err == nil {
				list = append(list, torr)
			}
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Timestamp > list[j].Timestamp
	})
	return list
}

func GetTorrent(hash metainfo.Hash) *TorrentDB {
	// Use read lock to prevent migration during read.
	dbMigrationLock.RLock()
	defer dbMigrationLock.RUnlock()

	if tdb == nil {
		return nil
	}

	buf := tdb.Get("Torrents", hash.HexString())
	if len(buf) == 0 {
		return nil
	}

	var torr *TorrentDB
	if err := json.Unmarshal(buf, &torr); err != nil {
		return nil
	}

	return torr
}

func RemTorrent(hash metainfo.Hash) {
	mu.Lock()
	defer mu.Unlock()

	tdb.Rem("Torrents", hash.HexString())
}
