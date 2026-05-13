package torr

import (
	"encoding/json"

	"server/settings"
	"server/torr/state"
	"server/torr/utils"

	"github.com/anacrolix/torrent/metainfo"
)

type TorrentDBStore interface {
	Add(*settings.TorrentDB)
	Get(metainfo.Hash) *settings.TorrentDB
	List() []*settings.TorrentDB
	Remove(metainfo.Hash)
}

type settingsTorrentDBStore struct{}
type noopTorrentDBStore struct{}
type tsFiles struct {
	TorrServer struct {
		Files []*state.TorrentFileStat `json:"Files"`
	} `json:"TorrServer"`
}

func NewSettingsTorrentDBStore() TorrentDBStore {
	return settingsTorrentDBStore{}
}

func NewNoopTorrentDBStore() TorrentDBStore {
	return noopTorrentDBStore{}
}

func torrentFromDBRecord(db *settings.TorrentDB) *Torrent {
	if db == nil {
		return nil
	}

	torr := new(Torrent)
	torr.TorrentSpec = db.TorrentSpec
	torr.Title = db.Title
	torr.Poster = db.Poster
	torr.Category = db.Category
	torr.Timestamp = db.Timestamp
	torr.Size = db.Size
	torr.Data = db.Data
	torr.Stat = state.TorrentInDB

	return torr
}

func torrentDBRecordFromTorrent(torr *Torrent) *settings.TorrentDB {
	if torr == nil {
		return nil
	}

	t := new(settings.TorrentDB)
	t.TorrentSpec = torr.TorrentSpec
	t.Title = torr.Title
	t.Category = torr.Category

	if torr.Data == "" {
		files := new(tsFiles)
		files.TorrServer.Files = torr.Status().FileStats

		buf, err := json.Marshal(files)
		if err == nil {
			t.Data = string(buf)
			torr.Data = t.Data
		}
	} else {
		t.Data = torr.Data
	}

	if torr.Poster != "" && utils.CheckImgURL(torr.Poster) {
		t.Poster = torr.Poster
	}

	t.Size = torr.Size
	if t.Size == 0 && torr.Torrent != nil {
		t.Size = torr.Torrent.Length()
	}
	// don't override timestamp from DB on edit
	t.Timestamp = torr.Timestamp

	return t
}

func (settingsTorrentDBStore) Add(torr *settings.TorrentDB) {
	settings.AddTorrent(torr)
}

func (settingsTorrentDBStore) Get(hash metainfo.Hash) *settings.TorrentDB {
	return settings.GetTorrent(hash)
}

func (settingsTorrentDBStore) List() []*settings.TorrentDB {
	return settings.ListTorrent()
}

func (settingsTorrentDBStore) Remove(hash metainfo.Hash) {
	settings.RemTorrent(hash)
}

func (noopTorrentDBStore) Add(*settings.TorrentDB) {}

func (noopTorrentDBStore) Get(metainfo.Hash) *settings.TorrentDB {
	return nil
}

func (noopTorrentDBStore) List() []*settings.TorrentDB {
	return nil
}

func (noopTorrentDBStore) Remove(metainfo.Hash) {}
