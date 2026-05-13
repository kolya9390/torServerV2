package apiservices

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/anacrolix/torrent"
	goffprobe "gopkg.in/vansante/go-ffprobe.v2"

	"server/ffprobe"
	"server/log"
	"server/modules"
	sets "server/settings"
	"server/torr"
	"server/torr/state"
	"server/torrshash"
	"server/torznab"
	"server/web/api"
	"server/web/api/utils"
)

type DefaultDeps struct {
	TorrentBackend    torr.TorrentService
	SettingsProvider  sets.SettingsProvider
	RuntimeSignals    torr.RuntimeSignals
	RuntimeController torr.RuntimeController
	RuntimeState      func() sets.RuntimeState
	ArgsProvider      sets.ArgsProvider
	SetViewed         func(*sets.Viewed)
	RemoveViewed      func(*sets.Viewed)
	ListViewed        func(string) []*sets.Viewed
}

type torrentService struct {
	backend        torr.TorrentService
	runtimeSignals torr.RuntimeSignals
}
type settingsService struct {
	provider          sets.SettingsProvider
	runtimeController torr.RuntimeController
	runtimeState      func() sets.RuntimeState
}
type viewedService struct {
	setViewed    func(*sets.Viewed)
	removeViewed func(*sets.Viewed)
	listViewed   func(string) []*sets.Viewed
}
type systemService struct {
	runtimeController torr.RuntimeController
}
type searchService struct {
	provider sets.SettingsProvider
}
type mediaService struct {
	runtimeState func() sets.RuntimeState
}
type modulesService struct {
	provider     sets.SettingsProvider
	argsProvider sets.ArgsProvider
}
type streamService struct{}
type playbackService struct{}

// NewDefault constructs the default API application services using runtime adapters.
func NewDefault() *api.APIServices {
	return NewDefaultWithDeps(DefaultDeps{})
}

func NewDefaultWithDeps(deps DefaultDeps) *api.APIServices {
	resolved := resolveDefaultDeps(deps)

	return &api.APIServices{
		Torrents: torrentService{
			backend:        resolved.TorrentBackend,
			runtimeSignals: resolved.RuntimeSignals,
		},
		Settings: settingsService{
			provider:          resolved.SettingsProvider,
			runtimeController: resolved.RuntimeController,
			runtimeState:      resolved.RuntimeState,
		},
		Viewed: viewedService{
			setViewed:    resolved.SetViewed,
			removeViewed: resolved.RemoveViewed,
			listViewed:   resolved.ListViewed,
		},
		System: systemService{runtimeController: resolved.RuntimeController},
		Search: searchService{provider: resolved.SettingsProvider},
		Media:  mediaService{runtimeState: resolved.RuntimeState},
		Modules: modulesService{
			provider:     resolved.SettingsProvider,
			argsProvider: resolved.ArgsProvider,
		},
		Streams:  streamService{},
		Playback: playbackService{},
	}
}

func resolveDefaultDeps(deps DefaultDeps) DefaultDeps {
	if deps.TorrentBackend == nil {
		deps.TorrentBackend = torr.NewNoopTorrentService()
	}

	if deps.SettingsProvider == nil {
		deps.SettingsProvider = sets.NewNoopSettingsProvider()
	}

	if deps.RuntimeSignals == nil {
		deps.RuntimeSignals = torr.NewNoopRuntimeSignals()
	}

	if deps.RuntimeController == nil {
		deps.RuntimeController = torr.NewNoopRuntimeController()
	}

	if deps.RuntimeState == nil {
		deps.RuntimeState = func() sets.RuntimeState { return sets.RuntimeState{} }
	}

	if deps.ArgsProvider == nil {
		deps.ArgsProvider = sets.NewNoopArgsProvider()
	}

	if deps.SetViewed == nil {
		deps.SetViewed = sets.SetViewed
	}

	if deps.RemoveViewed == nil {
		deps.RemoveViewed = sets.RemViewed
	}

	if deps.ListViewed == nil {
		deps.ListViewed = sets.ListViewed
	}

	return deps
}

func (d torrentService) Add(spec *torrent.TorrentSpec, title, poster, data, category string) (*torr.Torrent, error) {
	return d.backend.AddTorrent(spec, title, poster, data, category)
}

func (d torrentService) Get(hash string) *torr.Torrent {
	return d.backend.GetTorrent(hash)
}

func (d torrentService) Set(hash, title, poster, category, data string) *torr.Torrent {
	return d.backend.SetTorrent(hash, title, poster, category, data)
}

func (d torrentService) SaveToDB(tor *torr.Torrent) {
	d.backend.SaveTorrentDB(tor)
}

func (d torrentService) Remove(hash string) {
	d.backend.RemoveTorrent(hash)
}

func (d torrentService) List() []*torr.Torrent {
	return d.backend.ListTorrents()
}

func (d torrentService) Drop(hash string) {
	d.backend.DropTorrent(hash)
}

func (d torrentService) EnqueuePreload(tor *torr.Torrent, index int) bool {
	if tor == nil {
		return false
	}

	if signals := d.runtimeSignals; signals != nil {
		if signals.ActivePlaybackTorrents() > 1 {
			log.TLogln("EnqueuePreload: skip under multi-playback load")

			return false
		}

		if !signals.HasRuntimeBackend() && signals.ActiveStreams() > 1 {
			log.TLogln("EnqueuePreload: skip under multi-stream load", "active_streams=", signals.ActiveStreams())

			return false
		}
	}

	go tor.PreloadWithSettings(index, nil)

	return true
}

func (d torrentService) EnqueueMetadataFinalize(tor *torr.Torrent, spec *torrent.TorrentSpec, saveToDB bool) bool {
	if saveToDB {
		d.backend.SaveTorrentDB(tor)
	}

	return true
}

func (d torrentService) LoadFromDB(tor *torr.Torrent) *torr.Torrent {
	return d.backend.LoadTorrent(tor)
}

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

func (d systemService) Shutdown() {
	if d.runtimeController != nil {
		d.runtimeController.Shutdown()
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

func (d viewedService) SetViewed(v *sets.Viewed) {
	d.setViewed(v)
}

func (d viewedService) RemoveViewed(v *sets.Viewed) {
	d.removeViewed(v)
}

func (d viewedService) ListViewed(hash string) []*sets.Viewed {
	log.TLogln("viewedService.ListViewed: calling backend with hash:", hash)
	result := d.listViewed(hash)
	log.TLogln("viewedService.ListViewed: got result:", result)

	return result
}

func (d searchService) EnableTorznabSearch() bool {
	if d.provider == nil {
		return false
	}

	return d.provider.Get().SearchConfig().EnableTorznab
}

func (d searchService) TorznabSearch(query string, index int) []*torznab.TorrentDetails {
	return torznab.SearchWithProvider(query, index, d.provider)
}

func (d searchService) TorznabTest(host, key string) error {
	return torznab.Test(host, key)
}

func (d mediaService) ProbePlayURL(hash, fileID string) (*goffprobe.ProbeData, error) {
	serverCfg := d.runtimeState().ServerConfig()
	link := fmt.Sprintf("http://127.0.0.1:%s/play/%s/%s", serverCfg.Port, hash, fileID)
	if serverCfg.SSL {
		link = fmt.Sprintf("https://127.0.0.1:%s/play/%s/%s", serverCfg.SSLPort, hash, fileID)
	}

	return ffprobe.ProbeURL(link)
}

func (d modulesService) RestartDLNA(enable bool) error {
	return modules.RestartDLNAWithProviders(enable, d.provider, d.argsProvider)
}

func (d modulesService) StopDLNA() {
	modules.StopDLNA()
}

func (d streamService) ParseLink(link, title, poster, category string) (*torrent.TorrentSpec, api.StreamMeta, error) {
	if link == "" {
		return nil, api.StreamMeta{}, api.ErrStreamLinkEmpty
	}

	meta := api.StreamMeta{
		Title:    title,
		Poster:   poster,
		Category: category,
	}

	var err error

	link, err = url.QueryUnescape(link)
	if err != nil {
		return nil, meta, err
	}

	meta.Title, err = url.QueryUnescape(meta.Title)
	if err != nil {
		return nil, meta, err
	}

	meta.Poster, err = url.QueryUnescape(meta.Poster)
	if err != nil {
		return nil, meta, err
	}

	meta.Category, err = url.QueryUnescape(meta.Category)
	if err != nil {
		return nil, meta, err
	}

	var spec *torrent.TorrentSpec

	if strings.HasPrefix(link, "torrs://") || (len(link) > 45 && torrshash.IsBase62(link)) {
		var torrsHash *torrshash.TorrsHash

		spec, torrsHash, err = utils.ParseTorrsHash(link)
		if err != nil {
			return nil, api.StreamMeta{}, api.ErrStreamInvalidTorrsHash
		}

		if meta.Title == "" {
			meta.Title = torrsHash.Title()
		}

		if meta.Poster == "" {
			meta.Poster = torrsHash.Poster()
		}

		if meta.Category == "" {
			meta.Category = torrsHash.Category()
		}

		return spec, meta, nil
	}

	spec, err = utils.ParseLink(link)
	if err != nil {
		return nil, api.StreamMeta{}, api.ErrStreamInvalidLink
	}

	return spec, meta, nil
}

func (d streamService) EnsureTorrent(torrents api.TorrentService, spec *torrent.TorrentSpec, meta api.StreamMeta, allowCreate bool) (*torr.Torrent, error) {
	log.Debug("EnsureTorrent: starting", "hash", spec.InfoHash.HexString())
	log.Debug("EnsureTorrent: about to call torrents.Get")

	tor := torrents.Get(spec.InfoHash.HexString())
	log.Debug("EnsureTorrent: torrents.Get returned", "tor", tor != nil)

	if tor != nil {
		torStat := tor.Stat
		log.Debug("EnsureTorrent: found existing torrent", "stat", torStat)

		tMeta := tor.Metadata()
		if meta.Title == "" {
			meta.Title = tMeta.Title
		}

		if meta.Poster == "" {
			meta.Poster = tMeta.Poster
		}

		if meta.Category == "" {
			meta.Category = tMeta.Category
		}

		meta.Data = tMeta.Data
		// Torrent already in memory and working/preloading — skip GotInfo() to avoid deadlock.
		// The streaming layer (tor.Stream) will call GotInfo() internally if needed.
		if torStat == state.TorrentWorking || torStat == state.TorrentPreload {
			log.Debug("EnsureTorrent: torrent already working/preloading, skipping GotInfo")

			tor.EnsureTitleFromInfo()

			return tor, nil
		}

		if torStat == state.TorrentInDB {
			if !allowCreate {
				return nil, api.ErrStreamUnauthorized
			}

			log.Debug("EnsureTorrent: activating torrent from DB metadata")

			tor = torrents.LoadFromDB(tor)
			if tor == nil {
				return nil, api.ErrStreamConnectionTimeout
			}

			if tor.Stat == state.TorrentWorking || tor.Stat == state.TorrentPreload {
				tor.EnsureTitleFromInfo()

				return tor, nil
			}
		}
	}

	if tor == nil {
		log.Debug("EnsureTorrent: need to add torrent")

		if !allowCreate {
			return nil, api.ErrStreamUnauthorized
		}

		var err error

		tor, err = torrents.Add(spec, meta.Title, meta.Poster, meta.Data, meta.Category)
		if err != nil {
			log.Debug("EnsureTorrent: Add error", "error", err)

			return nil, err
		}

		log.Debug("EnsureTorrent: Add succeeded", "tor", tor)
	}

	log.Debug("EnsureTorrent: calling GotInfo")

	if !tor.GotInfo() {
		log.Debug("EnsureTorrent: no GotInfo, returning connection timeout")

		return nil, api.ErrStreamConnectionTimeout
	}

	log.Debug("EnsureTorrent: GotInfo succeeded")

	tor.EnsureTitleFromInfo()

	return tor, nil
}

func (d streamService) ParseFileIndex(index string, fileCount int) (int, error) {
	if fileCount == 1 {
		return 1, nil
	}

	ind, err := strconv.Atoi(index)
	if err != nil || ind < 0 {
		return 0, api.ErrStreamFileIndexInvalid
	}

	return ind, nil
}

func (d streamService) NormalizePlaylistName(rawName, fallback string) string {
	name := strings.ReplaceAll(rawName, `/`, "")
	if name == "" {
		return fallback + ".m3u"
	}

	if !strings.HasSuffix(strings.ToLower(name), ".m3u") && !strings.HasSuffix(strings.ToLower(name), ".m3u8") {
		return name + ".m3u"
	}

	return name
}
