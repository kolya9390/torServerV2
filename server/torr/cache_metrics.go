package torr

type CacheMetricsSnapshot struct {
	HotHits       uint64
	WarmHits      uint64
	Misses        uint64
	HotEvictions  uint64
	WarmEvictions uint64
}

func GetCacheMetricsSnapshot() CacheMetricsSnapshot {
	if bts == nil {
		return CacheMetricsSnapshot{}
	}
	list := bts.ListTorrents()
	var ret CacheMetricsSnapshot
	for _, t := range list {
		cache := t.GetCache()
		if cache == nil {
			continue
		}
		m := cache.Metrics()
		ret.HotHits += m.HotHits
		ret.WarmHits += m.WarmHits
		ret.Misses += m.Misses
		ret.HotEvictions += m.HotEvictions
		ret.WarmEvictions += m.WarmEvictions
	}
	return ret
}
