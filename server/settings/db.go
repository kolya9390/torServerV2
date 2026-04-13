package settings

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"server/log"

	bolt "go.etcd.io/bbolt"
)

type TDB struct {
	Path string
	db   *bolt.DB
}

var globalBboltDB TorrServerDB
var globalBboltDBMu sync.Mutex

func NewTDB() TorrServerDB {
	globalBboltDBMu.Lock()
	defer globalBboltDBMu.Unlock()

	if globalBboltDB != nil {
		return globalBboltDB // Return existing instance
	}

	db, err := bolt.Open(filepath.Join(Path, "config.db"), 0o666, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		log.TLogln(err)

		return nil
	}

	tdb := new(TDB)
	tdb.db = db
	tdb.Path = Path
	globalBboltDB = tdb

	return globalBboltDB
}

// GetRawDB returns the underlying *bolt.DB for direct access (used by auth package).
func (v *TDB) GetRawDB() any {
	return v.db
}

func (v *TDB) CloseDB() {
	if v.db != nil {
		if err := v.db.Close(); err != nil {
			log.TLogln("Error close db:", err)
		}

		v.db = nil
	}
}

func (v *TDB) Get(xpath, name string) []byte {
	if v == nil || v.db == nil {
		return nil
	}

	spath := strings.Split(xpath, "/")
	if len(spath) == 0 {
		return nil
	}

	var ret []byte

	err := v.db.View(func(tx *bolt.Tx) error {
		buckt := tx.Bucket([]byte(spath[0]))
		if buckt == nil {
			return nil
		}

		for i, p := range spath {
			if i == 0 {
				continue
			}

			buckt = buckt.Bucket([]byte(p))
			if buckt == nil {
				return nil
			}
		}

		data := buckt.Get([]byte(name))
		if data != nil {
			// CRITICAL: Copy the data before returning
			ret = make([]byte, len(data))
			copy(ret, data)
		}

		return nil
	})
	if err != nil {
		log.TLogln("Error get sets", xpath+"/"+name, ", error:", err)
	}

	return ret
}

func (v *TDB) Set(xpath, name string, value []byte) {
	if v == nil || v.db == nil {
		return
	}

	spath := strings.Split(xpath, "/")
	if len(spath) == 0 {
		return
	}

	err := v.db.Update(func(tx *bolt.Tx) error {
		buckt, err := tx.CreateBucketIfNotExists([]byte(spath[0]))
		if err != nil {
			return err
		}

		for i, p := range spath {
			if i == 0 {
				continue
			}

			buckt, err = buckt.CreateBucketIfNotExists([]byte(p))
			if err != nil {
				return err
			}
		}

		return buckt.Put([]byte(name), value)
	})
	if err != nil {
		log.TLogln("Error put sets", xpath+"/"+name, ", error:", err)
		log.TLogln("value:", value)
	}
}

func (v *TDB) List(xpath string) []string {
	if v == nil || v.db == nil {
		return nil
	}

	spath := strings.Split(xpath, "/")
	if len(spath) == 0 {
		return nil
	}

	var ret []string

	err := v.db.View(func(tx *bolt.Tx) error {
		buckt := tx.Bucket([]byte(spath[0]))
		if buckt == nil {
			return nil
		}

		for i, p := range spath {
			if i == 0 {
				continue
			}

			buckt = buckt.Bucket([]byte(p))
			if buckt == nil {
				return nil
			}
		}

		return buckt.ForEach(func(k, _ []byte) error {
			if len(k) > 0 {
				ret = append(ret, string(k))
			}

			return nil
		})
	})
	if err != nil {
		log.TLogln("Error list sets", xpath, ", error:", err)
	}

	return ret
}

func (v *TDB) Rem(xpath, name string) {
	if v == nil || v.db == nil {
		return
	}

	spath := strings.Split(xpath, "/")
	if len(spath) == 0 {
		return
	}

	err := v.db.Update(func(tx *bolt.Tx) error {
		buckt := tx.Bucket([]byte(spath[0]))
		if buckt == nil {
			return nil
		}

		for i, p := range spath {
			if i == 0 {
				continue
			}

			buckt = buckt.Bucket([]byte(p))
			if buckt == nil {
				return nil
			}
		}

		return buckt.Delete([]byte(name))
	})
	if err != nil {
		log.TLogln("Error rem sets", xpath+"/"+name, ", error:", err)
	}
}

func (v *TDB) Clear(xPath string) {
	if v == nil || v.db == nil {
		return
	}

	spath := strings.Split(xPath, "/")
	if len(spath) == 0 {
		return
	}

	err := v.db.Update(func(tx *bolt.Tx) error {
		buckt := tx.Bucket([]byte(spath[0]))
		if buckt == nil {
			return nil
		}

		for i, p := range spath {
			if i == 0 {
				continue
			}

			buckt = buckt.Bucket([]byte(p))
			if buckt == nil {
				return nil
			}
		}

		// Delete all entries in this bucket
		return buckt.ForEach(func(k, _ []byte) error {
			return buckt.Delete(k)
		})
	})

	if err != nil {
		log.TLogln("Error clear xPath", xPath, ", error:", err)
	}
}
