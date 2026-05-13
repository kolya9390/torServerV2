package settings

type CacheConfig struct {
	SizeBytes    int64
	ReadAheadPct int
	PreloadPct   int
	UseDisk      bool
	SavePath     string
	RemoveOnDrop bool
}

type NetworkConfig struct {
	ForceEncrypt        bool
	RetrackersMode      int
	EnableIPv6          bool
	DisableTCP          bool
	DisableUTP          bool
	DisableUPNP         bool
	DisableDHT          bool
	DisablePEX          bool
	DisableUpload       bool
	DownloadRateLimitKB int
	UploadRateLimitKB   int
	ConnectionsLimit    int
	PeersListenPort     int
}

type StreamConfig struct {
	ResponsiveMode       bool
	MaxConcurrentStreams int
	StreamQueueSize      int
	StreamQueueWaitSec   int
	AdaptiveRAMinMB      int
	AdaptiveRAMaxMB      int
}

type PlaybackConfig struct {
	DisconnectTimeoutSec int
	ReadAheadPct         int
	ResponsiveMode       bool
}

type DebugConfig struct {
	EnableDebug      bool
	ServiceOnlyDebug bool
}

type TLSConfig struct {
	Port int
	Cert string
	Key  string
}

type DLNAConfig struct {
	Enabled          bool
	FriendlyName     string
	ShowFSActiveTorr bool
}

type SearchConfig struct {
	EnableRutor   bool
	EnableTorznab bool
	TorznabURLs   []TorznabConfig
}

type ProxyConfig struct {
	Enabled bool
	Hosts   []string
}

type PersistenceConfig struct {
	SettingsInJSON bool
	ViewedInJSON   bool
}

type DiskCacheConfig struct {
	WarmSizeMB     int64
	WarmTTLMin     int
	SyncPolicy     string
	SyncIntervalMS int
	WriteBatchSize int
}

type WorkerConfig struct {
	MetadataWorkers   int
	MetadataQueueSize int
	PreloadWorkers    int
	PreloadQueueSize  int
}

func (s *BTSets) CacheConfig() CacheConfig {
	if s == nil {
		return CacheConfig{}
	}

	return CacheConfig{
		SizeBytes:    s.CacheSize,
		ReadAheadPct: s.ReaderReadAHead,
		PreloadPct:   s.PreloadCache,
		UseDisk:      s.UseDisk,
		SavePath:     s.TorrentsSavePath,
		RemoveOnDrop: s.RemoveCacheOnDrop,
	}
}

func (s *BTSets) NetworkConfig() NetworkConfig {
	if s == nil {
		return NetworkConfig{}
	}

	return NetworkConfig{
		ForceEncrypt:        s.ForceEncrypt,
		RetrackersMode:      s.RetrackersMode,
		EnableIPv6:          s.EnableIPv6,
		DisableTCP:          s.DisableTCP,
		DisableUTP:          s.DisableUTP,
		DisableUPNP:         s.DisableUPNP,
		DisableDHT:          s.DisableDHT,
		DisablePEX:          s.DisablePEX,
		DisableUpload:       s.DisableUpload,
		DownloadRateLimitKB: s.DownloadRateLimit,
		UploadRateLimitKB:   s.UploadRateLimit,
		ConnectionsLimit:    s.ConnectionsLimit,
		PeersListenPort:     s.PeersListenPort,
	}
}

func (s *BTSets) StreamConfig() StreamConfig {
	if s == nil {
		return StreamConfig{}
	}

	return StreamConfig{
		ResponsiveMode:       s.ResponsiveMode,
		MaxConcurrentStreams: s.MaxConcurrentStreams,
		StreamQueueSize:      s.StreamQueueSize,
		StreamQueueWaitSec:   s.StreamQueueWaitSec,
		AdaptiveRAMinMB:      s.AdaptiveRAMinMB,
		AdaptiveRAMaxMB:      s.AdaptiveRAMaxMB,
	}
}

func (s *BTSets) PlaybackConfig() PlaybackConfig {
	if s == nil {
		return PlaybackConfig{}
	}

	return PlaybackConfig{
		DisconnectTimeoutSec: s.TorrentDisconnectTimeout,
		ReadAheadPct:         s.ReaderReadAHead,
		ResponsiveMode:       s.ResponsiveMode,
	}
}

func (s *BTSets) DebugConfig() DebugConfig {
	if s == nil {
		return DebugConfig{}
	}

	return DebugConfig{
		EnableDebug:      s.EnableDebug,
		ServiceOnlyDebug: s.ServiceOnlyDebug,
	}
}

func (s *BTSets) TLSConfig() TLSConfig {
	if s == nil {
		return TLSConfig{}
	}

	return TLSConfig{
		Port: s.SslPort,
		Cert: s.SslCert,
		Key:  s.SslKey,
	}
}

func (s *BTSets) DLNAConfig() DLNAConfig {
	if s == nil {
		return DLNAConfig{}
	}

	return DLNAConfig{
		Enabled:          s.EnableDLNA,
		FriendlyName:     s.FriendlyName,
		ShowFSActiveTorr: s.ShowFSActiveTorr,
	}
}

func (s *BTSets) SearchConfig() SearchConfig {
	if s == nil {
		return SearchConfig{}
	}

	return SearchConfig{
		EnableRutor:   s.EnableRutorSearch,
		EnableTorznab: s.EnableTorznabSearch,
		TorznabURLs:   s.TorznabUrls,
	}
}

func (s *BTSets) ProxyConfig() ProxyConfig {
	if s == nil {
		return ProxyConfig{}
	}

	return ProxyConfig{
		Enabled: s.EnableProxy,
		Hosts:   s.ProxyHosts,
	}
}

func (s *BTSets) PersistenceConfig() PersistenceConfig {
	if s == nil {
		return PersistenceConfig{}
	}

	return PersistenceConfig{
		SettingsInJSON: s.StoreSettingsInJSON,
		ViewedInJSON:   s.StoreViewedInJSON,
	}
}

func (s *BTSets) DiskCacheConfig() DiskCacheConfig {
	if s == nil {
		return DiskCacheConfig{}
	}

	return DiskCacheConfig{
		WarmSizeMB:     s.WarmDiskCacheSizeMB,
		WarmTTLMin:     s.WarmDiskCacheTTLMin,
		SyncPolicy:     s.DiskSyncPolicy,
		SyncIntervalMS: s.DiskSyncIntervalMS,
		WriteBatchSize: s.DiskWriteBatchSize,
	}
}

func (s *BTSets) WorkerConfig() WorkerConfig {
	if s == nil {
		return WorkerConfig{}
	}

	return WorkerConfig{
		MetadataWorkers:   s.MetadataWorkers,
		MetadataQueueSize: s.MetadataQueueSize,
		PreloadWorkers:    s.PreloadWorkers,
		PreloadQueueSize:  s.PreloadQueueSize,
	}
}
