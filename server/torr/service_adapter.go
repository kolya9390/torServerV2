package torr

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"server/log"
	sets "server/settings"
)

// btserverAdapter adapts a concrete BTServer to TorrentService.
type btserverAdapter struct {
	bt *BTServer
}

type noopTorrentService struct{}

func NewNoopTorrentService() TorrentService {
	return noopTorrentService{}
}

// NewTorrentServiceWithBT creates a TorrentService bound to a concrete BTServer.
func NewTorrentServiceWithBT(bt *BTServer) TorrentService {
	return &btserverAdapter{bt: bt}
}

func (a *btserverAdapter) AddTorrent(spec *torrent.TorrentSpec, title, poster, data, category string) (*Torrent, error) {
	if a.bt == nil {
		return nil, ErrRuntimeUnavailable
	}

	torr, err := NewTorrent(spec, a.bt)
	if err != nil {
		log.TLogln("error add torrent:", err)

		return nil, err
	}

	torDB := torrentFromDBRecord(a.bt.currentDBStore().Get(spec.InfoHash))
	mergeTorrentMetadata(torr, torDB, title, poster, category, data)

	return torr, nil
}

func (a *btserverAdapter) GetTorrent(hash string) *Torrent {
	if a.bt == nil {
		return nil
	}

	infoHash := metainfo.NewHashFromHex(hash)
	if tor := a.bt.GetTorrent(infoHash); tor != nil {
		return tor
	}

	return torrentFromDBRecord(a.bt.currentDBStore().Get(infoHash))
}

func (a *btserverAdapter) SetTorrent(hash, title, poster, category, data string) *Torrent {
	if a.bt == nil {
		return nil
	}

	infoHash := metainfo.NewHashFromHex(hash)
	torr := a.bt.GetTorrent(infoHash)
	torrDB := torrentFromDBRecord(a.bt.currentDBStore().Get(infoHash))

	if title == "" && torr == nil && torrDB != nil {
		torr = a.GetTorrent(hash)
		if torr != nil {
			torr.GotInfo()
			if torr.Torrent != nil && torr.Info() != nil {
				title = torr.Info().Name
			}
		}
	}

	if torr != nil {
		if title == "" {
			torr.EnsureTitleFromInfo()
			title = torr.Title
		}

		torr.Title = title
		torr.Poster = poster
		torr.Category = category
		if data != "" {
			torr.Data = data
		}
	}

	if torrDB != nil {
		torrDB.Title = title
		torrDB.Poster = poster
		torrDB.Category = category
		if data != "" {
			torrDB.Data = data
		}
		if record := torrentDBRecordFromTorrent(torrDB); record != nil {
			a.bt.currentDBStore().Add(record)
		}
	}

	if torr != nil {
		return torr
	}

	return torrDB
}

func (a *btserverAdapter) RemoveTorrent(hash string) {
	if a.bt == nil {
		return
	}

	if sets.IsReadOnlyMode() {
		log.TLogln("API RemTorrent: Read-only DB mode!", hash)

		return
	}

	log.TLogln("API RemTorrent call", "hash=", hash, "active_streams=", GetActiveStreams())

	infoHash := metainfo.NewHashFromHex(hash)
	if a.bt.RemoveTorrent(infoHash) {
		cleanupTorrentDiskCache(hash, a.bt.currentSettings())
	}

	a.bt.currentDBStore().Remove(infoHash)
}

func (a *btserverAdapter) ListTorrents() []*Torrent {
	if a.bt == nil {
		return nil
	}

	btlist := a.bt.ListTorrents()
	for _, db := range a.bt.currentDBStore().List() {
		t := torrentFromDBRecord(db)
		if t == nil || t.TorrentSpec == nil {
			continue
		}

		hash := t.TorrentSpec.InfoHash
		if _, ok := btlist[hash]; !ok {
			btlist[hash] = t
		}
	}

	ret := make([]*Torrent, 0, len(btlist))
	for _, t := range btlist {
		ret = append(ret, t)
	}

	sort.Slice(ret, func(i, j int) bool {
		if ret[i].Timestamp != ret[j].Timestamp {
			return ret[i].Timestamp > ret[j].Timestamp
		}

		return ret[i].Title > ret[j].Title
	})

	return ret
}

func (a *btserverAdapter) DropTorrent(hash string) {
	if a.bt == nil {
		return
	}

	log.TLogln("API DropTorrent call", "hash=", hash, "active_streams=", GetActiveStreams())
	a.bt.RemoveTorrent(metainfo.NewHashFromHex(hash))
}

func (a *btserverAdapter) SaveTorrentDB(tor *Torrent) {
	if a.bt == nil {
		return
	}

	if record := torrentDBRecordFromTorrent(tor); record != nil {
		a.bt.currentDBStore().Add(record)
	}
}

func (a *btserverAdapter) LoadTorrent(tor *Torrent) *Torrent {
	if a.bt == nil || tor == nil || tor.TorrentSpec == nil {
		return nil
	}

	tr, err := NewTorrent(tor.TorrentSpec, a.bt)
	if err != nil {
		return nil
	}

	if !tr.WaitInfo() {
		return nil
	}

	tr.Title = tor.Title
	tr.Poster = tor.Poster
	tr.Data = tor.Data

	return tr
}

func mergeTorrentMetadata(torr, torDB *Torrent, title, poster, category, data string) {
	if torr == nil {
		return
	}

	torr.FillMissingMetadata(TorrentMetadata{
		Title:    title,
		Poster:   poster,
		Category: category,
		Data:     data,
	})

	if torDB != nil {
		torr.FillMissingMetadata(torDB.Metadata())
	}

	torr.EnsureTitleFromInfo()
}

func cleanupTorrentDiskCache(hash string, curSets *sets.BTSets) {
	if curSets == nil {
		return
	}

	cacheCfg := curSets.CacheConfig()
	if !cacheCfg.UseDisk || cacheCfg.SavePath == "" || hash == "" || hash == "/" {
		return
	}

	baseHash := filepath.Base(hash)
	if baseHash != hash || hash == "." || hash == ".." {
		log.TLogln("Skip unsafe cache cleanup hash:", hash)

		return
	}

	name := filepath.Join(cacheCfg.SavePath, hash)
	if err := os.RemoveAll(name); err != nil && !os.IsNotExist(err) {
		log.TLogln("Error remove cache:", err)
	}
}

func (noopTorrentService) AddTorrent(*torrent.TorrentSpec, string, string, string, string) (*Torrent, error) {
	return nil, ErrRuntimeUnavailable
}

func (noopTorrentService) GetTorrent(string) *Torrent {
	return nil
}

func (noopTorrentService) SetTorrent(string, string, string, string, string) *Torrent {
	return nil
}

func (noopTorrentService) RemoveTorrent(string) {}

func (noopTorrentService) ListTorrents() []*Torrent {
	return nil
}

func (noopTorrentService) DropTorrent(string) {}

func (noopTorrentService) SaveTorrentDB(*Torrent) {}

func (noopTorrentService) LoadTorrent(*Torrent) *Torrent {
	return nil
}
