package torr

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"server/log"
	sets "server/settings"
)

var bts *BTServer
var (
	shutdownHook   func()
	shutdownHookMu sync.RWMutex
)

func InitApiHelper(bt *BTServer) {
	bts = bt
	ensureJournalRecovered()
}

func SetShutdownHook(fn func()) {
	shutdownHookMu.Lock()
	defer shutdownHookMu.Unlock()
	shutdownHook = fn
}

func LoadTorrent(tor *Torrent) *Torrent {
	if tor.TorrentSpec == nil {
		return nil
	}
	tr, err := NewTorrent(tor.TorrentSpec, bts)
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

func AddTorrent(spec *torrent.TorrentSpec, title, poster string, data string, category string) (*Torrent, error) {
	torr, err := NewTorrent(spec, bts)
	if err != nil {
		log.TLogln("error add torrent:", err)
		return nil, err
	}

	torDB := GetTorrentDB(spec.InfoHash)

	if torr.Title == "" {
		torr.Title = title
		if title == "" && torDB != nil {
			torr.Title = torDB.Title
		}
		if torr.Title == "" && torr.Torrent != nil && torr.Info() != nil {
			torr.Title = torr.Info().Name
		}
	}

	if torr.Category == "" {
		torr.Category = category
		if torr.Category == "" && torDB != nil {
			torr.Category = torDB.Category
		}
	}

	if torr.Poster == "" {
		torr.Poster = poster
		if torr.Poster == "" && torDB != nil {
			torr.Poster = torDB.Poster
		}
	}

	if torr.Data == "" {
		torr.Data = data
		if torr.Data == "" && torDB != nil {
			torr.Data = torDB.Data
		}
	}

	return torr, nil
}

func SaveTorrentToDB(torr *Torrent) {
	jid := beginJournalOperation(journalOpSaveTorrentDB, snapshotTorrentDB(torr))
	log.TLogln("save to db:", torr.Hash())
	AddTorrentDB(torr)
	commitJournalOperation(jid, journalOpSaveTorrentDB, snapshotTorrentDB(torr))
}

func GetTorrent(hashHex string) *Torrent {
	if sets.BTsets == nil {
		return nil
	}
	hash, ok := parseHashHex(hashHex)
	if !ok {
		return nil
	}
	if sets.BTsets == nil || bts == nil {
		return nil
	}
	timeout := time.Second * time.Duration(sets.BTsets.TorrentDisconnectTimeout)
	if timeout > time.Minute {
		timeout = time.Minute
	}
	tor := bts.GetTorrent(hash)
	if tor != nil {
		// Проверяем есть ли Info у торрента
		if !tor.GotInfo() {
			// Ждем Info с таймаутом 10 секунд
			if !waitForInfo(tor, 10*time.Second) {
				log.TLogln("GetTorrent timeout waiting for info:", hashHex)
				return nil
			}
		}
		tor.AddExpiredTime(timeout)
		return tor
	}

	tr := GetTorrentDB(hash)
	if tr != nil {
		tor = tr
		if queued := EnqueueTorrentActivateFromDB(tor); !queued {
			log.TLogln("failed to enqueue torrent activate from DB", "hash", hashHex)
		}
	}
	return tor
}

// waitForInfo ждет получения Info от торрента с таймаутом
func waitForInfo(tor *Torrent, timeout time.Duration) bool {
	if tor == nil {
		return false
	}

	done := make(chan bool, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.TLogln("waitForInfo panic recovered", "panic", r)
				done <- false
			}
		}()
		done <- tor.GotInfo()
	}()

	select {
	case gotInfo := <-done:
		return gotInfo
	case <-time.After(timeout):
		return false
	}
}

func SetTorrent(hashHex, title, poster, category string, data string) *Torrent {
	if sets.BTsets == nil {
		return nil
	}
	hash, ok := parseHashHex(hashHex)
	if !ok {
		return nil
	}
	if bts == nil {
		torrDb := GetTorrentDB(hash)
		if torrDb != nil {
			torrDb.Title = title
			torrDb.Poster = poster
			torrDb.Category = category
			if data != "" {
				torrDb.Data = data
			}
			AddTorrentDB(torrDb)
		}
		return torrDb
	}
	torr := bts.GetTorrent(hash)
	torrDb := GetTorrentDB(hash)

	if title == "" && torr == nil && torrDb != nil {
		torr = GetTorrent(hashHex)
		if torr != nil {
			// Ждём Info с таймаутом 5 секунд
			if !waitForInfo(torr, 5*time.Second) {
				log.TLogln("SetTorrent timeout waiting for info:", hashHex)
			}
		}
		if torr != nil && torr.Torrent != nil && torr.Info() != nil {
			title = torr.Info().Name
		}
	}

	if torr != nil {
		if title == "" && torr.Torrent != nil && torr.Info() != nil {
			title = torr.Info().Name
		}
		torr.Title = title
		torr.Poster = poster
		torr.Category = category
		if data != "" {
			torr.Data = data
		}
	}
	// update torrent data in DB
	if torrDb != nil {
		torrDb.Title = title
		torrDb.Poster = poster
		torrDb.Category = category
		if data != "" {
			torrDb.Data = data
		}
		AddTorrentDB(torrDb)
	}
	if torr != nil {
		return torr
	} else {
		return torrDb
	}
}

func RemTorrent(hashHex string) {
	if sets.BTsets == nil {
		return
	}
	if sets.ReadOnly {
		log.TLogln("API RemTorrent: Read-only DB mode!", hashHex)
		return
	}
	hash, ok := parseHashHex(hashHex)
	if !ok {
		return
	}
	jid := beginJournalOperation(journalOpRemoveTorrentDB, hashPayload{Hash: hashHex})
	if bts != nil && bts.RemoveTorrent(hash) {
		if sets.BTsets != nil && sets.BTsets.UseDisk && hashHex != "" && hashHex != "/" {
			name := filepath.Join(sets.BTsets.TorrentsSavePath, hashHex)
			ff, _ := os.ReadDir(name)
			for _, f := range ff {
				if err := os.Remove(filepath.Join(name, f.Name())); err != nil {
					log.TLogln("failed to remove file in torrent dir", "file", f.Name(), "err", err)
				}
			}
			err := os.Remove(name)
			if err != nil {
				log.TLogln("Error remove cache:", err)
			}
		}
	}
	RemTorrentDB(hash)
	commitJournalOperation(jid, journalOpRemoveTorrentDB, hashPayload{Hash: hashHex})
}

func ListTorrent() []*Torrent {
	if sets.BTsets == nil {
		return []*Torrent{}
	}
	if bts == nil {
		dblist := ListTorrentsDB()
		ret := make([]*Torrent, 0, len(dblist))
		for _, t := range dblist {
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

	// Get torrent list with timeout to avoid blocking
	btlist := make(map[metainfo.Hash]*Torrent)
	done := make(chan struct{}, 1)
	go func() {
		btlist = bts.ListTorrents()
		close(done)
	}()

	select {
	case <-done:
		// Got the list successfully
	case <-time.After(5 * time.Second):
		log.TLogln("ListTorrent timeout - returning cached list from DB")
		btlist = make(map[metainfo.Hash]*Torrent)
	}

	dblist := ListTorrentsDB()

	for hash, t := range dblist {
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
		} else {
			return ret[i].Title > ret[j].Title
		}
	})

	return ret
}

func DropTorrent(hashHex string) {
	if sets.BTsets == nil {
		return
	}
	hash, ok := parseHashHex(hashHex)
	if !ok {
		return
	}
	if bts == nil {
		return
	}
	jid := beginJournalOperation(journalOpDropTorrent, hashPayload{Hash: hashHex})
	bts.RemoveTorrent(hash)
	commitJournalOperation(jid, journalOpDropTorrent, hashPayload{Hash: hashHex})
}

func parseHashHex(hashHex string) (hash metainfo.Hash, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	hash = metainfo.NewHashFromHex(hashHex)
	return hash, true
}

func SetSettings(set *sets.BTSets) {
	if sets.ReadOnly {
		log.TLogln("API SetSettings: Read-only DB mode!")
		return
	}
	setCopy := cloneBTSets(set)
	jid := beginJournalOperation(journalOpSetSettings, setCopy)
	sets.SetBTSets(set)
	log.TLogln("drop all torrents")
	dropAllTorrent()
	time.Sleep(time.Second * 1)
	log.TLogln("disconect")
	bts.Disconnect()
	log.TLogln("connect")
	if err := bts.Connect(); err != nil {
		log.TLogln("connect error:", err)
	}
	time.Sleep(time.Second * 1)
	log.TLogln("end set settings")
	commitJournalOperation(jid, journalOpSetSettings, setCopy)
}

func SetDefSettings() {
	if sets.ReadOnly {
		log.TLogln("API SetDefSettings: Read-only DB mode!")
		return
	}
	jid := beginJournalOperation(journalOpSetDefaultSettings, nil)
	sets.SetDefaultConfig()
	log.TLogln("drop all torrents")
	dropAllTorrent()
	time.Sleep(time.Second * 1)
	log.TLogln("disconect")
	bts.Disconnect()
	log.TLogln("connect")
	if err := bts.Connect(); err != nil {
		log.TLogln("connect error:", err)
	}
	time.Sleep(time.Second * 1)
	log.TLogln("end set default settings")
	commitJournalOperation(jid, journalOpSetDefaultSettings, nil)
}

func SetStoragePreferences(prefs map[string]interface{}) error {
	jid := beginJournalOperation(journalOpSetStoragePrefs, prefs)
	err := sets.SetStoragePreferences(prefs)
	if err != nil {
		return err
	}
	commitJournalOperation(jid, journalOpSetStoragePrefs, prefs)
	return nil
}

func dropAllTorrent() {
	for _, torr := range bts.torrents {
		torr.drop()
		<-torr.closed
	}
}

func Shutdown() {
	shutdownHookMu.RLock()
	hook := shutdownHook
	shutdownHookMu.RUnlock()
	if hook != nil {
		log.TLogln("Received shutdown. Stopping server")
		hook()
		return
	}

	bts.Disconnect()
	sets.CloseDB()
	log.TLogln("Received shutdown. Resources closed")
}

func WriteStatus(w io.Writer) {
	bts.client.WriteStatus(w)
}

func Preload(torr *Torrent, index int) {
	cache := float32(sets.BTsets.CacheSize)
	preload := float32(sets.BTsets.PreloadCache)
	size := int64((cache / 100.0) * preload)
	if size <= 0 {
		return
	}
	if size > sets.BTsets.CacheSize {
		size = sets.BTsets.CacheSize
	}
	torr.Preload(index, size)
}
