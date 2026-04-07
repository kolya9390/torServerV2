package settings

import (
	"sync"
)

// StaticConfig holds configuration loaded from config.yaml at startup.
// This is immutable after startup - do not modify at runtime.
type StaticConfig struct {
	// Server
	Port     string
	SSL      bool
	SSLCert  string
	SSLKey   string
	HTTPAuth bool
	SearchWA bool

	// DLNA
	DLNAEnabled      bool
	DLNAFriendlyName string

	// Cache
	CacheSizeMB     int64
	PreloadCache    int
	ReaderReadAHead int

	// Torrent
	ForceEncrypt             bool
	RetrackersMode           int
	TorrentDisconnectTimeout int

	// Network
	EnableIPv6        bool
	DisableTCP        bool
	DisableUTP        bool
	DisableUPNP       bool
	DisableDHT        bool
	DisablePEX        bool
	DisableUpload     bool
	ConnectionsLimit  int
	DownloadRateLimit int
	UploadRateLimit   int
	PeersListenPort   int

	// Search
	EnableRutorSearch   bool
	EnableTorznabSearch bool

	// TMDB
	TMDBAPIKey string

	// Debug
	EnableDebug bool
}

// staticConfig is the global static configuration loaded from config.yaml.
var staticConfig StaticConfig
var staticConfigMu sync.RWMutex

// SetStaticConfig sets the static configuration (called once at startup).
func SetStaticConfig(cfg StaticConfig) {
	staticConfigMu.Lock()
	defer staticConfigMu.Unlock()

	staticConfig = cfg
}

// GetStaticConfig returns the static configuration.
func GetStaticConfig() StaticConfig {
	staticConfigMu.RLock()
	defer staticConfigMu.RUnlock()

	return staticConfig
}
