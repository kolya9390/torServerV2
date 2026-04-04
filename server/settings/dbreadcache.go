package settings

import (
	"sync"

	"server/log"
)

type DBReadCache struct {
	db             TorrServerDB
	listCache      map[string][]string
	listCacheMutex sync.RWMutex
	dataCache      map[[2]string][]byte
	dataCacheMutex sync.RWMutex
}

func cloneBytes(data []byte) []byte {
	if data == nil {
		return nil
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	return cp
}

func cloneStrings(items []string) []string {
	if items == nil {
		return nil
	}
	cp := make([]string, len(items))
	copy(cp, items)
	return cp
}

func NewDBReadCache(db TorrServerDB) TorrServerDB {
	cdb := &DBReadCache{
		db:        db,
		listCache: map[string][]string{},
		dataCache: map[[2]string][]byte{},
	}
	return cdb
}

func (v *DBReadCache) CloseDB() {
	v.db.CloseDB()
	v.db = nil
	v.listCache = nil
	v.dataCache = nil
}

func (v *DBReadCache) Get(xPath, name string) []byte {
	if v == nil {
		return nil
	}
	if v.dataCache == nil {
		return nil
	}
	cacheKey := v.makeDataCacheKey(xPath, name)

	v.dataCacheMutex.RLock()
	if data, ok := v.dataCache[cacheKey]; ok {
		defer v.dataCacheMutex.RUnlock()
		return cloneBytes(data)
	}
	v.dataCacheMutex.RUnlock()

	if v.db == nil {
		return nil
	}
	data := v.db.Get(xPath, name)

	v.dataCacheMutex.Lock()
	if v.dataCache != nil {
		v.dataCache[cacheKey] = cloneBytes(data)
	}
	v.dataCacheMutex.Unlock()

	return cloneBytes(data)
}

func (v *DBReadCache) Set(xPath, name string, value []byte) {
	if v == nil {
		return
	}
	if ReadOnly {
		if IsDebug() {
			log.TLogln("DBReadCache.Set: Read-only DB mode!", name)
		}
		return
	}
	if v.dataCache == nil || v.db == nil {
		log.TLogln("DBReadCache.Set: dataCache or db is nil, cannot set", name)
		return
	}

	cacheKey := v.makeDataCacheKey(xPath, name)

	v.dataCacheMutex.Lock()
	if v.dataCache != nil {
		v.dataCache[cacheKey] = cloneBytes(value)
	}
	v.dataCacheMutex.Unlock()

	v.listCacheMutex.Lock()
	if v.listCache != nil {
		delete(v.listCache, xPath)
	}
	v.listCacheMutex.Unlock()

	v.db.Set(xPath, name, value)
}

func (v *DBReadCache) List(xPath string) []string {
	if v == nil {
		return nil
	}
	if v.listCache == nil {
		return nil
	}

	v.listCacheMutex.RLock()
	if names, ok := v.listCache[xPath]; ok {
		defer v.listCacheMutex.RUnlock()
		return cloneStrings(names)
	}
	v.listCacheMutex.RUnlock()

	if v.db == nil {
		return nil
	}

	names := v.db.List(xPath)

	v.listCacheMutex.Lock()
	if v.listCache != nil {
		v.listCache[xPath] = cloneStrings(names)
	}
	v.listCacheMutex.Unlock()

	return cloneStrings(names)
}

func (v *DBReadCache) Rem(xPath, name string) {
	if v == nil {
		return
	}
	if ReadOnly {
		if IsDebug() {
			log.TLogln("DBReadCache.Rem: Read-only DB mode!", name)
		}
		return
	}
	if v.dataCache == nil || v.db == nil {
		log.TLogln("DBReadCache.Rem: no dataCache or DB is closed, cannot remove", name)
		return
	}

	cacheKey := v.makeDataCacheKey(xPath, name)

	v.dataCacheMutex.Lock()
	if v.dataCache != nil {
		delete(v.dataCache, cacheKey)
	}
	v.dataCacheMutex.Unlock()

	v.listCacheMutex.Lock()
	if v.listCache != nil {
		delete(v.listCache, xPath)
	}
	v.listCacheMutex.Unlock()

	v.db.Rem(xPath, name)
}

func (v *DBReadCache) Clear(xPath string) {
	if v == nil {
		return
	}
	if ReadOnly {
		if IsDebug() {
			log.TLogln("DBReadCache.Clear: Read-only DB mode!", xPath)
		}
		return
	}

	// Clear from underlying DB first
	if v.db != nil {
		v.db.Clear(xPath)
	}

	// Clear cache
	if v.listCache != nil {
		v.listCacheMutex.Lock()
		delete(v.listCache, xPath)
		v.listCacheMutex.Unlock()
	}

	// Clear data cache entries for this xPath
	if v.dataCache != nil {
		v.dataCacheMutex.Lock()
		for key := range v.dataCache {
			if key[0] == xPath {
				delete(v.dataCache, key)
			}
		}
		v.dataCacheMutex.Unlock()
	}
}

func (v *DBReadCache) makeDataCacheKey(xPath, name string) [2]string {
	return [2]string{xPath, name}
}
