package settings

// SettingsProvider defines the interface for accessing and modifying application settings.
// This abstraction allows dependency injection and easier testing.
type SettingsProvider interface {
	// Get returns the current settings snapshot (merged static + dynamic).
	Get() *BTSets

	// Set updates the dynamic settings at runtime.
	Set(sets *BTSets)

	// ReadOnly returns true if settings cannot be modified.
	ReadOnly() bool

	// GetStaticConfig returns the static configuration from config.yaml.
	GetStaticConfig() StaticConfig

	// GetStoragePreferences returns the current storage preferences.
	GetStoragePreferences() map[string]any
}

// DefaultSettingsProvider is the global SettingsProvider instance.
var DefaultSettingsProvider SettingsProvider = &btsetsProvider{}

// btsetsProvider is the default SettingsProvider implementation
// that merges static config with dynamic BTSets.
type btsetsProvider struct{}

func (p *btsetsProvider) Get() *BTSets {
	btsetsMu.RLock()
	defer btsetsMu.RUnlock()

	if BTsets != nil {
		return BTsets
	}

	// Fallback to static config if BTSets is nil
	staticCfg := GetStaticConfig()

	return staticToBTSets(staticCfg)
}

func (p *btsetsProvider) Set(sets *BTSets) {
	btsetsMu.Lock()
	defer btsetsMu.Unlock()

	BTsets = sets
}

func (p *btsetsProvider) ReadOnly() bool {
	return ReadOnly
}

func (p *btsetsProvider) GetStaticConfig() StaticConfig {
	return GetStaticConfig()
}

func (p *btsetsProvider) GetStoragePreferences() map[string]any {
	btsetsMu.RLock()
	defer btsetsMu.RUnlock()

	if BTsets == nil {
		return map[string]any{}
	}

	return map[string]any{
		"settingsInJSON": BTsets.StoreSettingsInJSON,
		"viewedInJSON":   BTsets.StoreViewedInJSON,
	}
}

// staticToBTSets converts StaticConfig to BTSets for fallback.
func staticToBTSets(cfg StaticConfig) *BTSets {
	return &BTSets{
		CacheSize:                cfg.CacheSizeMB * 1024 * 1024,
		PreloadCache:             cfg.PreloadCache,
		ReaderReadAHead:          cfg.ReaderReadAHead,
		ForceEncrypt:             cfg.ForceEncrypt,
		RetrackersMode:           cfg.RetrackersMode,
		TorrentDisconnectTimeout: cfg.TorrentDisconnectTimeout,
		EnableDebug:              cfg.EnableDebug,
		EnableDLNA:               cfg.DLNAEnabled,
		FriendlyName:             cfg.DLNAFriendlyName,
		EnableRutorSearch:        cfg.EnableRutorSearch,
		EnableTorznabSearch:      cfg.EnableTorznabSearch,
		TMDBSettings: TMDBConfig{
			APIKey: cfg.TMDBAPIKey,
		},
		EnableIPv6:        cfg.EnableIPv6,
		DisableTCP:        cfg.DisableTCP,
		DisableUTP:        cfg.DisableUTP,
		DisableUPNP:       cfg.DisableUPNP,
		DisableDHT:        cfg.DisableDHT,
		DisablePEX:        cfg.DisablePEX,
		DisableUpload:     cfg.DisableUpload,
		ConnectionsLimit:  cfg.ConnectionsLimit,
		DownloadRateLimit: cfg.DownloadRateLimit,
		UploadRateLimit:   cfg.UploadRateLimit,
		PeersListenPort:   cfg.PeersListenPort,
	}
}

// SetSettingsProvider allows replacing the default provider for testing.
func SetSettingsProvider(provider SettingsProvider) {
	if provider != nil {
		DefaultSettingsProvider = provider
	}
}
