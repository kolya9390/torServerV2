package config

import "server/settings"

// ToStaticConfig converts Config to settings.StaticConfig.
func (c *Config) ToStaticConfig() settings.StaticConfig {
	return settings.StaticConfig{
		// Server
		Port:     c.Server.Port,
		SSL:      c.Server.SSL,
		SSLCert:  c.Server.SSLCert,
		SSLKey:   c.Server.SSLKey,
		HTTPAuth: c.Server.HTTPAuth,
		SearchWA: c.Server.SearchWA,

		// DLNA
		DLNAEnabled:      c.DLNA.Enabled,
		DLNAFriendlyName: c.DLNA.FriendlyName,

		// Cache
		CacheSizeMB:     c.Cache.SizeMB,
		PreloadCache:    c.Cache.PreloadPercent,
		ReaderReadAHead: 0,

		// Torrent
		ForceEncrypt:             c.Torrent.ForceEncrypt,
		RetrackersMode:           c.Torrent.RetrackersMode,
		TorrentDisconnectTimeout: c.Torrent.DisconnectTimeoutSec,

		// Network
		EnableIPv6:        c.Network.EnableIPv6,
		DisableTCP:        c.Network.DisableTCP,
		DisableUTP:        c.Network.DisableUTP,
		DisableUPNP:       c.Network.DisableUPNP,
		DisableDHT:        c.Network.DisableDHT,
		DisablePEX:        c.Network.DisablePEX,
		DisableUpload:     c.Network.DisableUpload,
		ConnectionsLimit:  c.Torrent.ConnectionsLimit,
		DownloadRateLimit: c.Network.DownloadRateLimitKB,
		UploadRateLimit:   c.Network.UploadRateLimitKB,
		PeersListenPort:   c.Network.PeersListenPort,

		// Search
		EnableRutorSearch:   c.Search.EnableRutor,
		EnableTorznabSearch: c.Search.EnableTorznab,

		// TMDB
		TMDBAPIKey: c.TMDB.APIKey,

		// Debug
		EnableDebug: c.Debug.Enabled,
	}
}
