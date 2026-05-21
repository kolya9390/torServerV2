package apiservices

import (
	"fmt"

	sets "server/settings"
)

func (d settingsService) Current() *sets.BTSets {
	return d.provider.Get()
}

func (d settingsService) currentSettings() *sets.BTSets {
	return d.provider.Get()
}

func (d settingsService) currentSearchConfig() sets.SearchConfig {
	return d.currentSettings().SearchConfig()
}

func (d settingsService) currentNetworkConfig() sets.NetworkConfig {
	return d.currentSettings().NetworkConfig()
}

func (d settingsService) currentCacheConfig() sets.CacheConfig {
	return d.currentSettings().CacheConfig()
}

func (d settingsService) currentProxyConfig() sets.ProxyConfig {
	return d.currentSettings().ProxyConfig()
}

func (d settingsService) currentDLNAConfig() sets.DLNAConfig {
	return d.currentSettings().DLNAConfig()
}

func (d settingsService) currentTLSConfig() sets.TLSConfig {
	return d.currentSettings().TLSConfig()
}

func (d settingsService) Set(v *sets.BTSets) {
	if d.runtimeController != nil {
		d.runtimeController.ApplySettings(v)
	}
}

func (d settingsService) SetDefault() {
	if d.runtimeController != nil {
		d.runtimeController.ResetDefaultSettings()
	}
}

func (d settingsService) ReadOnly() bool {
	return d.provider.ReadOnly()
}

func (d settingsService) GetStoragePreferences() map[string]any {
	return d.provider.GetStoragePreferences()
}

func (d settingsService) SetStoragePreferences(prefs map[string]any) error {
	return nil
}

func (d settingsService) TMDBConfig() (sets.TMDBConfig, bool) {
	return d.currentSettings().TMDBSettings, true
}

func (d settingsService) BuildPlayURL(hash, fileID string) string {
	serverCfg := d.runtimeState().ServerConfig()
	link := fmt.Sprintf("http://127.0.0.1:%s/play/%s/%s", serverCfg.Port, hash, fileID)

	if serverCfg.SSL {
		link = fmt.Sprintf("https://127.0.0.1:%s/play/%s/%s", serverCfg.SSLPort, hash, fileID)
	}

	return link
}

func (d settingsService) EnableDLNA() bool {
	return d.currentDLNAConfig().Enabled
}

func (d settingsService) EnableDebug() bool {
	return d.currentSettings().DebugConfig().EnableDebug
}

func (d settingsService) EnableIPv6() bool {
	return d.currentNetworkConfig().EnableIPv6
}

func (d settingsService) DisableTCP() bool {
	return d.currentNetworkConfig().DisableTCP
}

func (d settingsService) DisableUTP() bool {
	return d.currentNetworkConfig().DisableUTP
}

func (d settingsService) DisableUPNP() bool {
	return d.currentNetworkConfig().DisableUPNP
}

func (d settingsService) DisableDHT() bool {
	return d.currentNetworkConfig().DisableDHT
}

func (d settingsService) DisablePEX() bool {
	return d.currentNetworkConfig().DisablePEX
}

func (d settingsService) DisableUpload() bool {
	return d.currentNetworkConfig().DisableUpload
}

func (d settingsService) ForceEncrypt() bool {
	return d.currentNetworkConfig().ForceEncrypt
}

func (d settingsService) RetrackersMode() int {
	return d.currentNetworkConfig().RetrackersMode
}

func (d settingsService) DownloadRateLimit() int {
	return d.currentNetworkConfig().DownloadRateLimitKB
}

func (d settingsService) UploadRateLimit() int {
	return d.currentNetworkConfig().UploadRateLimitKB
}

func (d settingsService) ConnectionsLimit() int {
	return d.currentNetworkConfig().ConnectionsLimit
}

func (d settingsService) PeersListenPort() int {
	return d.currentNetworkConfig().PeersListenPort
}

func (d settingsService) CacheSize() int64 {
	return d.currentCacheConfig().SizeBytes
}

func (d settingsService) PreloadCache() int {
	return d.currentCacheConfig().PreloadPct
}

func (d settingsService) UseDisk() bool {
	return d.currentCacheConfig().UseDisk
}

func (d settingsService) TorrentsSavePath() string {
	return d.currentCacheConfig().SavePath
}

func (d settingsService) EnableRutorSearch() bool {
	return d.currentSearchConfig().EnableRutor
}

func (d settingsService) EnableTorznabSearch() bool {
	return d.currentSearchConfig().EnableTorznab
}

func (d settingsService) TorznabURLs() []sets.TorznabConfig {
	return d.currentSearchConfig().TorznabURLs
}

func (d settingsService) EnableProxy() bool {
	return d.currentProxyConfig().Enabled
}

func (d settingsService) ProxyHosts() []string {
	return d.currentProxyConfig().Hosts
}

func (d settingsService) SslCert() string {
	return d.currentTLSConfig().Cert
}

func (d settingsService) SslKey() string {
	return d.currentTLSConfig().Key
}

func (d settingsService) SslPort() int {
	return d.currentTLSConfig().Port
}

func (d settingsService) FriendlyName() string {
	return d.currentDLNAConfig().FriendlyName
}

func (d settingsService) ShowFSActiveTorr() bool {
	return d.currentDLNAConfig().ShowFSActiveTorr
}

func (d settingsService) TorrentDisconnectTimeout() int {
	return d.currentSettings().PlaybackConfig().DisconnectTimeoutSec
}

func (d settingsService) TorrentsDir() string {
	return d.runtimeState().PathConfig().Path
}
