package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"server/log"
	"server/settings"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	DLNA      DLNAConfig      `yaml:"dlna"`
	Cache     CacheConfig     `yaml:"cache"`
	Torrent   TorrentConfig   `yaml:"torrent"`
	Network   NetworkConfig   `yaml:"network"`
	Search    SearchConfig    `yaml:"search"`
	TMDB      TMDBConfig      `yaml:"tmdb"`
	Stream    StreamConfig    `yaml:"streaming"`
	DiskCache DiskCacheConfig `yaml:"disk_cache"`
	Workers   WorkersConfig   `yaml:"workers"`
	Debug     DebugConfig     `yaml:"debug"`
	Proxy     ProxyConfig     `yaml:"proxy"`
	Storage   StorageConfig   `yaml:"storage"`
}

type ServerConfig struct {
	Port     string `yaml:"port"`
	SSL      bool   `yaml:"ssl"`
	SSLPort  string `yaml:"ssl_port"`
	SSLCert  string `yaml:"ssl_cert"`
	SSLKey   string `yaml:"ssl_key"`
	HTTPAuth bool   `yaml:"http_auth"`
	SearchWA bool   `yaml:"search_wa"`
}

type DLNAConfig struct {
	Enabled      bool   `yaml:"enabled"`
	FriendlyName string `yaml:"friendly_name"`
}

type CacheConfig struct {
	SizeMB           int64  `yaml:"size_mb"`
	PreloadPercent   int    `yaml:"preload_percent"`
	UseDisk          bool   `yaml:"use_disk"`
	TorrentsSavePath string `yaml:"torrents_save_path"`
}

type TorrentConfig struct {
	ForceEncrypt         bool `yaml:"force_encrypt"`
	RetrackersMode       int  `yaml:"retrackers_mode"`
	DisconnectTimeoutSec int  `yaml:"disconnect_timeout_sec"`
	ConnectionsLimit     int  `yaml:"connections_limit"`
}

type NetworkConfig struct {
	EnableIPv6          bool `yaml:"enable_ipv6"`
	DisableTCP          bool `yaml:"disable_tcp"`
	DisableUTP          bool `yaml:"disable_utp"`
	DisableUPNP         bool `yaml:"disable_upnp"`
	DisableDHT          bool `yaml:"disable_dht"`
	DisablePEX          bool `yaml:"disable_pex"`
	DisableUpload       bool `yaml:"disable_upload"`
	DownloadRateLimitKB int  `yaml:"download_rate_limit_kb"`
	UploadRateLimitKB   int  `yaml:"upload_rate_limit_kb"`
	PeersListenPort     int  `yaml:"peers_listen_port"`
}

type SearchConfig struct {
	EnableRutor   bool           `yaml:"enable_rutor"`
	EnableTorznab bool           `yaml:"enable_torznab"`
	TorznabURLs   []TorznabEntry `yaml:"torznab_urls"`
}

type TorznabEntry struct {
	Host string `yaml:"host"`
	Key  string `yaml:"key"`
	Name string `yaml:"name"`
}

type TMDBConfig struct {
	APIKey     string `yaml:"api_key"`
	APIURL     string `yaml:"api_url"`
	ImageURL   string `yaml:"image_url"`
	ImageURLRu string `yaml:"image_url_ru"`
}

type StreamConfig struct {
	ResponsiveMode       bool   `yaml:"responsive_mode"`
	CoreProfile          string `yaml:"core_profile"`
	MaxConcurrentStreams int    `yaml:"max_concurrent_streams"`
	StreamQueueSize      int    `yaml:"stream_queue_size"`
	StreamQueueWaitSec   int    `yaml:"stream_queue_wait_sec"`
	AdaptiveRAMinMB      int    `yaml:"adaptive_ra_min_mb"`
	AdaptiveRAMaxMB      int    `yaml:"adaptive_ra_max_mb"`
	ReadAheadPercent     int    `yaml:"read_ahead_percent"`
}

type DiskCacheConfig struct {
	WarmSizeMB     int64  `yaml:"warm_size_mb"`
	WarmTTLMin     int    `yaml:"warm_ttl_min"`
	SyncPolicy     string `yaml:"sync_policy"`
	SyncIntervalMS int    `yaml:"sync_interval_ms"`
	WriteBatchSize int    `yaml:"write_batch_size"`
}

type WorkersConfig struct {
	MetadataWorkers   int `yaml:"metadata_workers"`
	MetadataQueueSize int `yaml:"metadata_queue_size"`
	PreloadWorkers    int `yaml:"preload_workers"`
	PreloadQueueSize  int `yaml:"preload_queue_size"`
}

type DebugConfig struct {
	Enabled          bool `yaml:"enabled"`
	ShowFSActiveTorr bool `yaml:"show_fs_active_torr"`
}

type ProxyConfig struct {
	Enabled bool     `yaml:"enabled"`
	Hosts   []string `yaml:"hosts"`
}

type StorageConfig struct {
	SettingsInJSON bool `yaml:"settings_in_json"`
	ViewedInJSON   bool `yaml:"viewed_in_json"`
}

var loadedConfig *Config

func Load(configPath string) (*Config, error) {
	if configPath == "" {
		configPath = findConfigFile()
	}

	cfg := &Config{}

	if configPath != "" && fileExists(configPath) {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}

		log.TLogln("Loaded configuration from:", configPath)
	} else {
		log.TLogln("Using default configuration (no config.yml found)")
	}

	applyDefaults(cfg)
	loadedConfig = cfg
	return cfg, nil
}

func Get() *Config {
	if loadedConfig == nil {
		loadedConfig = &Config{}
		applyDefaults(loadedConfig)
	}
	return loadedConfig
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func findConfigFile() string {
	searchPaths := []string{
		"config.yml",
		"./config.yml",
		"../config.yml",
		"/etc/torrserver/config.yml",
		os.Getenv("TS_CONFIG"),
	}

	for _, path := range searchPaths {
		if path != "" && fileExists(path) {
			return path
		}
	}

	return ""
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Port == "" {
		cfg.Server.Port = "8090"
	}
	if cfg.Server.SSLPort == "" {
		cfg.Server.SSLPort = "8091"
	}

	if cfg.Cache.SizeMB == 0 {
		cfg.Cache.SizeMB = 64
	}
	if cfg.Cache.PreloadPercent == 0 {
		cfg.Cache.PreloadPercent = 50
	}

	if cfg.Torrent.RetrackersMode == 0 {
		cfg.Torrent.RetrackersMode = 1
	}
	if cfg.Torrent.DisconnectTimeoutSec == 0 {
		cfg.Torrent.DisconnectTimeoutSec = 30
	}
	if cfg.Torrent.ConnectionsLimit == 0 {
		cfg.Torrent.ConnectionsLimit = 25
	}

	if cfg.Stream.CoreProfile == "" {
		cfg.Stream.CoreProfile = "custom"
	}
	if cfg.Stream.StreamQueueWaitSec == 0 {
		cfg.Stream.StreamQueueWaitSec = 3
	}
	if cfg.Stream.AdaptiveRAMinMB == 0 {
		cfg.Stream.AdaptiveRAMinMB = 4
	}
	if cfg.Stream.AdaptiveRAMaxMB == 0 {
		cfg.Stream.AdaptiveRAMaxMB = 64
	}
	if cfg.Stream.ReadAheadPercent == 0 {
		cfg.Stream.ReadAheadPercent = 95
	}

	if cfg.DiskCache.SyncPolicy == "" {
		cfg.DiskCache.SyncPolicy = "periodic"
	}
	if cfg.DiskCache.SyncIntervalMS == 0 {
		cfg.DiskCache.SyncIntervalMS = 1000
	}
	if cfg.DiskCache.WriteBatchSize == 0 {
		cfg.DiskCache.WriteBatchSize = 16
	}
	if cfg.DiskCache.WarmTTLMin == 0 {
		cfg.DiskCache.WarmTTLMin = 180
	}

	if cfg.TMDB.APIURL == "" {
		cfg.TMDB.APIURL = "https://api.themoviedb.org"
	}
	if cfg.TMDB.ImageURL == "" {
		cfg.TMDB.ImageURL = "https://image.tmdb.org"
	}
	if cfg.TMDB.ImageURLRu == "" {
		cfg.TMDB.ImageURLRu = "https://imagetmdb.com"
	}

	if cfg.DLNA.FriendlyName == "" {
		cfg.DLNA.FriendlyName = "TorrServer"
	}

	if len(cfg.Proxy.Hosts) == 0 {
		cfg.Proxy.Hosts = []string{"*themoviedb.org", "*tmdb.org", "rutor.info"}
	}
}

func (c *Config) GetConfigPath() string {
	execPath, _ := os.Executable()
	return filepath.Join(filepath.Dir(execPath), "config.yml")
}

func (c *Config) ApplyToBTSets(sets *settings.BTSets) {
	if c == nil || sets == nil {
		return
	}

	sets.CacheSize = c.Cache.SizeMB * 1024 * 1024
	sets.PreloadCache = c.Cache.PreloadPercent
	sets.UseDisk = c.Cache.UseDisk
	sets.TorrentsSavePath = c.Cache.TorrentsSavePath

	sets.ForceEncrypt = c.Torrent.ForceEncrypt
	sets.RetrackersMode = c.Torrent.RetrackersMode
	sets.TorrentDisconnectTimeout = c.Torrent.DisconnectTimeoutSec
	sets.ConnectionsLimit = c.Torrent.ConnectionsLimit

	sets.EnableIPv6 = c.Network.EnableIPv6
	sets.DisableTCP = c.Network.DisableTCP
	sets.DisableUTP = c.Network.DisableUTP
	sets.DisableUPNP = c.Network.DisableUPNP
	sets.DisableDHT = c.Network.DisableDHT
	sets.DisablePEX = c.Network.DisablePEX
	sets.DisableUpload = c.Network.DisableUpload
	sets.DownloadRateLimit = c.Network.DownloadRateLimitKB
	sets.UploadRateLimit = c.Network.UploadRateLimitKB
	sets.PeersListenPort = c.Network.PeersListenPort

	sets.EnableDLNA = c.DLNA.Enabled
	sets.FriendlyName = c.DLNA.FriendlyName

	sets.EnableRutorSearch = c.Search.EnableRutor
	sets.EnableTorznabSearch = c.Search.EnableTorznab
	sets.TorznabUrls = make([]settings.TorznabConfig, len(c.Search.TorznabURLs))
	for i, url := range c.Search.TorznabURLs {
		sets.TorznabUrls[i] = settings.TorznabConfig{
			Host: url.Host,
			Key:  url.Key,
			Name: url.Name,
		}
	}

	sets.TMDBSettings = settings.TMDBConfig{
		APIKey:     c.TMDB.APIKey,
		APIURL:     c.TMDB.APIURL,
		ImageURL:   c.TMDB.ImageURL,
		ImageURLRu: c.TMDB.ImageURLRu,
	}

	sets.ResponsiveMode = c.Stream.ResponsiveMode
	sets.CoreProfile = c.Stream.CoreProfile
	sets.MaxConcurrentStreams = c.Stream.MaxConcurrentStreams
	sets.StreamQueueSize = c.Stream.StreamQueueSize
	sets.StreamQueueWaitSec = c.Stream.StreamQueueWaitSec
	sets.AdaptiveRAMinMB = c.Stream.AdaptiveRAMinMB
	sets.AdaptiveRAMaxMB = c.Stream.AdaptiveRAMaxMB
	sets.ReaderReadAHead = c.Stream.ReadAheadPercent

	sets.WarmDiskCacheSizeMB = c.DiskCache.WarmSizeMB
	sets.WarmDiskCacheTTLMin = c.DiskCache.WarmTTLMin
	sets.DiskSyncPolicy = c.DiskCache.SyncPolicy
	sets.DiskSyncIntervalMS = c.DiskCache.SyncIntervalMS
	sets.DiskWriteBatchSize = c.DiskCache.WriteBatchSize

	sets.MetadataWorkers = c.Workers.MetadataWorkers
	sets.MetadataQueueSize = c.Workers.MetadataQueueSize
	sets.PreloadWorkers = c.Workers.PreloadWorkers
	sets.PreloadQueueSize = c.Workers.PreloadQueueSize

	sets.EnableDebug = c.Debug.Enabled
	sets.ShowFSActiveTorr = c.Debug.ShowFSActiveTorr

	sets.EnableProxy = c.Proxy.Enabled
	sets.ProxyHosts = c.Proxy.Hosts

	sets.StoreSettingsInJson = c.Storage.SettingsInJSON
	sets.StoreViewedInJson = c.Storage.ViewedInJSON

	if c.Server.SSLPort != "" {
		if port, err := parsePort(c.Server.SSLPort); err == nil {
			sets.SslPort = port
		}
	}
	sets.SslCert = c.Server.SSLCert
	sets.SslKey = c.Server.SSLKey
}

func parsePort(s string) (int, error) {
	var port int
	_, err := fmt.Sscanf(s, "%d", &port)
	return port, err
}
