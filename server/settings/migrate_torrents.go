package settings

import (
	"os"
	"path/filepath"
	"time"

	"server/internal/torrentparse"
	"server/log"

	bolt "go.etcd.io/bbolt"
)

var dbTorrentsName = []byte("Torrents")

type torrentBackupDB struct {
	Name      string
	Magnet    string
	InfoBytes []byte
	Hash      string
	Size      int64
	Timestamp int64
	Category  string
	Data      string
}

// loadTorrentFromBucket reads a single torrent from the old BBolt bucket.
// Returns nil if the torrent is incomplete.
func loadTorrentFromBucket(hdb *bolt.Bucket, hash string) *torrentBackupDB {
	torr := &torrentBackupDB{Hash: hash}

	required := map[string]func([]byte){
		"Name": func(v []byte) { torr.Name = string(v) },
		"Link": func(v []byte) { torr.Magnet = string(v) },
		"Size": func(v []byte) { torr.Size = b2i(v) },
	}

	for key, setter := range required {
		val := hdb.Get([]byte(key))
		if val == nil {
			log.TLogln("MigrateTorrents: missing required field", key, "for hash", hash)

			return nil
		}

		setter(val)
	}

	// Optional fields (may not exist in old DB)
	if val := hdb.Get([]byte("Timestamp")); val != nil {
		torr.Timestamp = b2i(val)
	}

	if val := hdb.Get([]byte("Category")); val != nil {
		torr.Category = string(val)
	}

	if val := hdb.Get([]byte("Data")); val != nil {
		torr.Data = string(val)
	}

	return torr
}

// MigrateTorrents migrates torrents from torrserver.db to config.db.
// Also migrates Category and Data fields if they exist in the old DB.
func MigrateTorrent() {
	MigrateTorrentAtPath(currentRuntimePath())
}

func MigrateTorrentAtPath(runtimePath string) {
	if runtimePath == "" {
		runtimePath = "."
	}

	srcPath := filepath.Join(runtimePath, "torrserver.db")
	if _, err := os.Lstat(srcPath); os.IsNotExist(err) {
		return
	}

	db, err := bolt.Open(srcPath, 0o666, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		log.TLogln("MigrateTorrents: open source db error:", err)

		return
	}

	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.TLogln("MigrateTorrents close source db error:", closeErr)
		}
	}()

	torrs := loadAllTorrents(db)

	if len(torrs) == 0 {
		log.TLogln("MigrateTorrents: no torrents found in old db")

		return
	}

	migrateAndSave(torrs)

	if err := os.Remove(srcPath); err != nil && !os.IsNotExist(err) {
		log.TLogln("MigrateTorrents remove source db error:", err)
	}
}

// loadAllTorrents reads all torrents from the old BBolt database.
func loadAllTorrents(db *bolt.DB) []*torrentBackupDB {
	var torrs []*torrentBackupDB

	err := db.View(func(tx *bolt.Tx) error {
		tdb := tx.Bucket(dbTorrentsName)
		if tdb == nil {
			return nil
		}

		c := tdb.Cursor()

		for h, _ := c.First(); h != nil; h, _ = c.Next() {
			hdb := tdb.Bucket(h)
			if hdb == nil {
				continue
			}

			if torr := loadTorrentFromBucket(hdb, string(h)); torr != nil {
				torrs = append(torrs, torr)
			}
		}

		return nil
	})
	if err != nil {
		log.TLogln("MigrateTorrents: read error:", err)
	}

	return torrs
}

// migrateAndSave converts and saves old-format torrents to new format.
func migrateAndSave(torrs []*torrentBackupDB) {
	saved := 0
	skipped := 0

	for _, oldTorr := range torrs {
		spec, err := torrentparse.ParseLink(oldTorr.Magnet)
		if err != nil {
			log.TLogln("MigrateTorrents: parse link error, skipping", oldTorr.Name)

			skipped++

			continue
		}

		title := oldTorr.Name
		if len(spec.DisplayName) > len(title) {
			title = spec.DisplayName
		}

		log.TLogln("Migrate torrent:", oldTorr.Name, "hash:", oldTorr.Hash, "size:", oldTorr.Size,
			"category:", oldTorr.Category)

		newTorr := &TorrentDB{
			TorrentSpec: spec,
			Title:       title,
			Category:    oldTorr.Category,
			Data:        oldTorr.Data,
			Timestamp:   oldTorr.Timestamp,
			Size:        oldTorr.Size,
		}

		AddTorrent(newTorr)

		saved++
	}

	log.TLogln("MigrateTorrents: saved", saved, "torrents, skipped", skipped)
}
