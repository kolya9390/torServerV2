package apiservices

import (
	"server/internal/app/contracts"
	sets "server/settings"
)

func mapSettingsFromBTSets(src *sets.BTSets) *contracts.Settings {
	if src == nil {
		return nil
	}

	return &contracts.Settings{
		CacheSize:                       src.CacheSize,
		ReaderReadAHead:                 src.ReaderReadAHead,
		PreloadCache:                    src.PreloadCache,
		UseDisk:                         src.UseDisk,
		TorrentsSavePath:                src.TorrentsSavePath,
		RemoveCacheOnDrop:               src.RemoveCacheOnDrop,
		ForceEncrypt:                    src.ForceEncrypt,
		RetrackersMode:                  src.RetrackersMode,
		TorrentDisconnectTimeout:        src.TorrentDisconnectTimeout,
		EnableDebug:                     src.EnableDebug,
		ServiceOnlyDebug:                src.ServiceOnlyDebug,
		DebugEstablishedConnsOverride:   src.DebugEstablishedConnsOverride,
		DebugTotalHalfOpenConnsOverride: src.DebugTotalHalfOpenConnsOverride,
		DebugTrackerBudgetOverride:      src.DebugTrackerBudgetOverride,
		DebugStablePeerCap:              src.DebugStablePeerCap,
		EnableDLNA:                      src.EnableDLNA,
		FriendlyName:                    src.FriendlyName,
		EnableRutorSearch:               src.EnableRutorSearch,
		EnableTorznabSearch:             src.EnableTorznabSearch,
		TorznabUrls:                     mapTorznabConfigFromSettings(src.TorznabUrls),
		TMDBSettings:                    mapTMDBConfigFromSettings(src.TMDBSettings),
		EnableIPv6:                      src.EnableIPv6,
		DisableTCP:                      src.DisableTCP,
		DisableUTP:                      src.DisableUTP,
		DisableUPNP:                     src.DisableUPNP,
		DisableDHT:                      src.DisableDHT,
		DisablePEX:                      src.DisablePEX,
		DisableUpload:                   src.DisableUpload,
		DownloadRateLimit:               src.DownloadRateLimit,
		UploadRateLimit:                 src.UploadRateLimit,
		ConnectionsLimit:                src.ConnectionsLimit,
		PeersListenPort:                 src.PeersListenPort,
		EnableLPD:                       src.EnableLPD,
		LPDIPv6:                         src.LPDIPv6,
		SslPort:                         src.SslPort,
		SslCert:                         src.SslCert,
		SslKey:                          src.SslKey,
		ResponsiveMode:                  src.ResponsiveMode,
		CoreProfile:                     src.CoreProfile,
		MaxConcurrentStreams:            src.MaxConcurrentStreams,
		StreamQueueSize:                 src.StreamQueueSize,
		StreamQueueWaitSec:              src.StreamQueueWaitSec,
		MaxUniquePlaybackTorrents:       src.MaxUniquePlaybackTorrents,
		AdaptiveRAMinMB:                 src.AdaptiveRAMinMB,
		AdaptiveRAMaxMB:                 src.AdaptiveRAMaxMB,
		WarmDiskCacheSizeMB:             src.WarmDiskCacheSizeMB,
		WarmDiskCacheTTLMin:             src.WarmDiskCacheTTLMin,
		DiskSyncPolicy:                  src.DiskSyncPolicy,
		DiskSyncIntervalMS:              src.DiskSyncIntervalMS,
		DiskWriteBatchSize:              src.DiskWriteBatchSize,
		MetadataWorkers:                 src.MetadataWorkers,
		MetadataQueueSize:               src.MetadataQueueSize,
		PreloadWorkers:                  src.PreloadWorkers,
		PreloadQueueSize:                src.PreloadQueueSize,
		ShowFSActiveTorr:                src.ShowFSActiveTorr,
		StoreSettingsInJSON:             src.StoreSettingsInJSON,
		StoreViewedInJSON:               src.StoreViewedInJSON,
		EnableProxy:                     src.EnableProxy,
		ProxyHosts:                      append([]string(nil), src.ProxyHosts...),
	}
}

func mapSettingsToBTSets(src *contracts.Settings) *sets.BTSets {
	if src == nil {
		return nil
	}

	return &sets.BTSets{
		CacheSize:                       src.CacheSize,
		ReaderReadAHead:                 src.ReaderReadAHead,
		PreloadCache:                    src.PreloadCache,
		UseDisk:                         src.UseDisk,
		TorrentsSavePath:                src.TorrentsSavePath,
		RemoveCacheOnDrop:               src.RemoveCacheOnDrop,
		ForceEncrypt:                    src.ForceEncrypt,
		RetrackersMode:                  src.RetrackersMode,
		TorrentDisconnectTimeout:        src.TorrentDisconnectTimeout,
		EnableDebug:                     src.EnableDebug,
		ServiceOnlyDebug:                src.ServiceOnlyDebug,
		DebugEstablishedConnsOverride:   src.DebugEstablishedConnsOverride,
		DebugTotalHalfOpenConnsOverride: src.DebugTotalHalfOpenConnsOverride,
		DebugTrackerBudgetOverride:      src.DebugTrackerBudgetOverride,
		DebugStablePeerCap:              src.DebugStablePeerCap,
		EnableDLNA:                      src.EnableDLNA,
		FriendlyName:                    src.FriendlyName,
		EnableRutorSearch:               src.EnableRutorSearch,
		EnableTorznabSearch:             src.EnableTorznabSearch,
		TorznabUrls:                     mapTorznabConfigToSettings(src.TorznabUrls),
		TMDBSettings:                    mapTMDBConfigToSettings(src.TMDBSettings),
		EnableIPv6:                      src.EnableIPv6,
		DisableTCP:                      src.DisableTCP,
		DisableUTP:                      src.DisableUTP,
		DisableUPNP:                     src.DisableUPNP,
		DisableDHT:                      src.DisableDHT,
		DisablePEX:                      src.DisablePEX,
		DisableUpload:                   src.DisableUpload,
		DownloadRateLimit:               src.DownloadRateLimit,
		UploadRateLimit:                 src.UploadRateLimit,
		ConnectionsLimit:                src.ConnectionsLimit,
		PeersListenPort:                 src.PeersListenPort,
		EnableLPD:                       src.EnableLPD,
		LPDIPv6:                         src.LPDIPv6,
		SslPort:                         src.SslPort,
		SslCert:                         src.SslCert,
		SslKey:                          src.SslKey,
		ResponsiveMode:                  src.ResponsiveMode,
		CoreProfile:                     src.CoreProfile,
		MaxConcurrentStreams:            src.MaxConcurrentStreams,
		StreamQueueSize:                 src.StreamQueueSize,
		StreamQueueWaitSec:              src.StreamQueueWaitSec,
		MaxUniquePlaybackTorrents:       src.MaxUniquePlaybackTorrents,
		AdaptiveRAMinMB:                 src.AdaptiveRAMinMB,
		AdaptiveRAMaxMB:                 src.AdaptiveRAMaxMB,
		WarmDiskCacheSizeMB:             src.WarmDiskCacheSizeMB,
		WarmDiskCacheTTLMin:             src.WarmDiskCacheTTLMin,
		DiskSyncPolicy:                  src.DiskSyncPolicy,
		DiskSyncIntervalMS:              src.DiskSyncIntervalMS,
		DiskWriteBatchSize:              src.DiskWriteBatchSize,
		MetadataWorkers:                 src.MetadataWorkers,
		MetadataQueueSize:               src.MetadataQueueSize,
		PreloadWorkers:                  src.PreloadWorkers,
		PreloadQueueSize:                src.PreloadQueueSize,
		ShowFSActiveTorr:                src.ShowFSActiveTorr,
		StoreSettingsInJSON:             src.StoreSettingsInJSON,
		StoreViewedInJSON:               src.StoreViewedInJSON,
		EnableProxy:                     src.EnableProxy,
		ProxyHosts:                      append([]string(nil), src.ProxyHosts...),
	}
}

func mapTMDBConfigFromSettings(src sets.TMDBConfig) contracts.TMDBConfig {
	return contracts.TMDBConfig{
		APIKey:     src.APIKey,
		APIURL:     src.APIURL,
		ImageURL:   src.ImageURL,
		ImageURLRu: src.ImageURLRu,
	}
}

func mapTMDBConfigToSettings(src contracts.TMDBConfig) sets.TMDBConfig {
	return sets.TMDBConfig{
		APIKey:     src.APIKey,
		APIURL:     src.APIURL,
		ImageURL:   src.ImageURL,
		ImageURLRu: src.ImageURLRu,
	}
}

func mapTorznabConfigFromSettings(src []sets.TorznabConfig) []contracts.TorznabConfig {
	if len(src) == 0 {
		return nil
	}

	mapped := make([]contracts.TorznabConfig, 0, len(src))

	for _, item := range src {
		mapped = append(mapped, contracts.TorznabConfig{
			Host: item.Host,
			Key:  item.Key,
			Name: item.Name,
		})
	}

	return mapped
}

func mapTorznabConfigToSettings(src []contracts.TorznabConfig) []sets.TorznabConfig {
	if len(src) == 0 {
		return nil
	}

	mapped := make([]sets.TorznabConfig, 0, len(src))

	for _, item := range src {
		mapped = append(mapped, sets.TorznabConfig{
			Host: item.Host,
			Key:  item.Key,
			Name: item.Name,
		})
	}

	return mapped
}
