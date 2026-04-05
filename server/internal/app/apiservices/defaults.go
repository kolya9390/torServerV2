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

type torrentService struct{}
type settingsService struct{}
type viewedService struct{}
type systemService struct{}
type searchService struct{}
type mediaService struct{}
type modulesService struct{}
type streamService struct{}
type playbackService struct{}

// NewDefault constructs the default API application services using runtime adapters.
func NewDefault() *api.APIServices {
	return &api.APIServices{
		Torrents: torrentService{},
		Settings: settingsService{},
		Viewed:   viewedService{},
		System:   systemService{},
		Search:   searchService{},
		Media:    mediaService{},
		Modules:  modulesService{},
		Streams:  streamService{},
		Playback: playbackService{},
	}
}

func (d torrentService) Add(spec *torrent.TorrentSpec, title, poster, data, category string) (*torr.Torrent, error) {
	return torr.AddTorrent(spec, title, poster, data, category)
}

func (d torrentService) Get(hash string) *torr.Torrent {
	return torr.GetTorrent(hash)
}

func (d torrentService) Set(hash, title, poster, category, data string) *torr.Torrent {
	return torr.SetTorrent(hash, title, poster, category, data)
}

func (d torrentService) SaveToDB(tor *torr.Torrent) {
	torr.SaveTorrentToDB(tor)
}

func (d torrentService) Remove(hash string) {
	torr.RemTorrent(hash)
}

func (d torrentService) List() []*torr.Torrent {
	return torr.ListTorrent()
}

func (d torrentService) Drop(hash string) {
	torr.DropTorrent(hash)
}

func (d torrentService) EnqueuePreload(tor *torr.Torrent, index int) bool {
	torr.Preload(tor, index)
	return true
}

func (d torrentService) EnqueueMetadataFinalize(tor *torr.Torrent, spec *torrent.TorrentSpec, saveToDB bool) bool {
	if saveToDB {
		torr.SaveTorrentToDB(tor)
	}
	return true
}

func (d torrentService) LoadFromDB(tor *torr.Torrent) *torr.Torrent {
	return torr.LoadTorrent(tor)
}

func (d settingsService) Current() *sets.BTSets {
	return sets.BTsets
}

func (d settingsService) Set(v *sets.BTSets) {
	torr.SetSettings(v)
}

func (d settingsService) SetDefault() {
	torr.SetDefSettings()
}

func (d settingsService) ReadOnly() bool {
	return sets.ReadOnly
}

func (d settingsService) GetStoragePreferences() map[string]interface{} {
	return sets.GetStoragePreferences()
}

func (d settingsService) SetStoragePreferences(prefs map[string]interface{}) error {
	return nil
}

func (d settingsService) TMDBConfig() (sets.TMDBConfig, bool) {
	if sets.BTsets == nil {
		return sets.TMDBConfig{}, false
	}
	return sets.BTsets.TMDBSettings, true
}

func (d settingsService) BuildPlayURL(hash, fileID string) string {
	link := fmt.Sprintf("http://127.0.0.1:%s/play/%s/%s", sets.Port, hash, fileID)
	if sets.Ssl {
		link = fmt.Sprintf("https://127.0.0.1:%s/play/%s/%s", sets.SslPort, hash, fileID)
	}
	return link
}

func (d settingsService) EnableDLNA() bool {
	return sets.BTsets != nil && sets.BTsets.EnableDLNA
}

func (d settingsService) EnableDebug() bool {
	return sets.BTsets != nil && sets.BTsets.EnableDebug
}

func (d settingsService) EnableIPv6() bool {
	return sets.BTsets == nil || sets.BTsets.EnableIPv6
}

func (d settingsService) DisableTCP() bool {
	return sets.BTsets != nil && sets.BTsets.DisableTCP
}

func (d settingsService) DisableUTP() bool {
	return sets.BTsets != nil && sets.BTsets.DisableUTP
}

func (d settingsService) DisableUPNP() bool {
	return sets.BTsets != nil && sets.BTsets.DisableUPNP
}

func (d settingsService) DisableDHT() bool {
	return sets.BTsets != nil && sets.BTsets.DisableDHT
}

func (d settingsService) DisablePEX() bool {
	return sets.BTsets != nil && sets.BTsets.DisablePEX
}

func (d settingsService) DisableUpload() bool {
	return sets.BTsets != nil && sets.BTsets.DisableUpload
}

func (d settingsService) ForceEncrypt() bool {
	return sets.BTsets != nil && sets.BTsets.ForceEncrypt
}

func (d settingsService) RetrackersMode() int {
	if sets.BTsets == nil {
		return 1
	}
	return sets.BTsets.RetrackersMode
}

func (d settingsService) DownloadRateLimit() int {
	if sets.BTsets == nil {
		return 0
	}
	return sets.BTsets.DownloadRateLimit
}

func (d settingsService) UploadRateLimit() int {
	if sets.BTsets == nil {
		return 0
	}
	return sets.BTsets.UploadRateLimit
}

func (d settingsService) ConnectionsLimit() int {
	if sets.BTsets == nil {
		return 25
	}
	return sets.BTsets.ConnectionsLimit
}

func (d settingsService) PeersListenPort() int {
	if sets.BTsets == nil {
		return 0
	}
	return sets.BTsets.PeersListenPort
}

func (d settingsService) CacheSize() int64 {
	if sets.BTsets == nil {
		return 64 * 1024 * 1024
	}
	return sets.BTsets.CacheSize
}

func (d settingsService) PreloadCache() int {
	if sets.BTsets == nil {
		return 50
	}
	return sets.BTsets.PreloadCache
}

func (d settingsService) UseDisk() bool {
	return sets.BTsets != nil && sets.BTsets.UseDisk
}

func (d settingsService) TorrentsSavePath() string {
	if sets.BTsets == nil {
		return ""
	}
	return sets.BTsets.TorrentsSavePath
}

func (d settingsService) EnableRutorSearch() bool {
	return sets.BTsets != nil && sets.BTsets.EnableRutorSearch
}

func (d settingsService) EnableTorznabSearch() bool {
	return sets.BTsets != nil && sets.BTsets.EnableTorznabSearch
}

func (d settingsService) TorznabURLs() []sets.TorznabConfig {
	if sets.BTsets == nil {
		return nil
	}
	return sets.BTsets.TorznabUrls
}

func (d settingsService) EnableProxy() bool {
	return sets.BTsets != nil && sets.BTsets.EnableProxy
}

func (d settingsService) ProxyHosts() []string {
	if sets.BTsets == nil {
		return nil
	}
	return sets.BTsets.ProxyHosts
}

func (d settingsService) SslCert() string {
	if sets.BTsets == nil {
		return ""
	}
	return sets.BTsets.SslCert
}

func (d settingsService) SslKey() string {
	if sets.BTsets == nil {
		return ""
	}
	return sets.BTsets.SslKey
}

func (d settingsService) SslPort() int {
	if sets.BTsets == nil {
		return 0
	}
	return sets.BTsets.SslPort
}

func (d settingsService) FriendlyName() string {
	if sets.BTsets == nil {
		return "TorrServer"
	}
	return sets.BTsets.FriendlyName
}

func (d settingsService) ShowFSActiveTorr() bool {
	return sets.BTsets == nil || sets.BTsets.ShowFSActiveTorr
}

func (d settingsService) TorrentDisconnectTimeout() int {
	if sets.BTsets == nil {
		return 30
	}
	return sets.BTsets.TorrentDisconnectTimeout
}

func (d settingsService) TorrentsDir() string {
	return sets.Path
}

func (d viewedService) SetViewed(v *sets.Viewed) {
	sets.SetViewed(v)
}

func (d viewedService) RemoveViewed(v *sets.Viewed) {
	sets.RemViewed(v)
}

func (d viewedService) ListViewed(hash string) []*sets.Viewed {
	log.TLogln("viewedService.ListViewed: calling sets.ListViewed with hash:", hash)
	result := sets.ListViewed(hash)
	log.TLogln("viewedService.ListViewed: got result:", result)
	return result
}

func (d systemService) Shutdown() {
	torr.Shutdown()
}

func (d searchService) EnableTorznabSearch() bool {
	return sets.BTsets != nil && sets.BTsets.EnableTorznabSearch
}

func (d searchService) TorznabSearch(query string, index int) []*torznab.TorrentDetails {
	return torznab.Search(query, index)
}

func (d searchService) TorznabTest(host, key string) error {
	return torznab.Test(host, key)
}

func (d mediaService) ProbePlayURL(hash, fileID string) (*goffprobe.ProbeData, error) {
	link := settingsService{}.BuildPlayURL(hash, fileID)
	return ffprobe.ProbeUrl(link)
}

func (d modulesService) RestartDLNA(enable bool) error {
	return modules.RestartDLNA(enable)
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
	link, _ = url.QueryUnescape(link)
	meta.Title, _ = url.QueryUnescape(meta.Title)
	meta.Poster, _ = url.QueryUnescape(meta.Poster)
	meta.Category, _ = url.QueryUnescape(meta.Category)

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
	log.TLogln("[DEBUG] EnsureTorrent: starting, hash=", spec.InfoHash.HexString())
	log.TLogln("[DEBUG] EnsureTorrent: about to call torrents.Get")
	tor := torrents.Get(spec.InfoHash.HexString())
	log.TLogln("[DEBUG] EnsureTorrent: torrents.Get returned, tor=", tor != nil)
	if tor != nil {
		torStat := tor.Stat
		log.TLogln("[DEBUG] EnsureTorrent: found existing torrent, stat=", torStat)
		if meta.Title == "" {
			meta.Title = tor.Title
		}
		if meta.Poster == "" {
			meta.Poster = tor.Poster
		}
		if meta.Category == "" {
			meta.Category = tor.Category
		}
		meta.Data = tor.Data
		// Torrent already in memory and working/preloading — skip GotInfo() to avoid deadlock.
		// The streaming layer (tor.Stream) will call GotInfo() internally if needed.
		if torStat == state.TorrentWorking || torStat == state.TorrentPreload {
			log.TLogln("[DEBUG] EnsureTorrent: torrent already working/preloading, skipping GotInfo")
			if tor.Title == "" {
				tor.Title = tor.Name()
			}
			return tor, nil
		}
	}

	if tor == nil {
		log.TLogln("[DEBUG] EnsureTorrent: need to add torrent")
		if !allowCreate {
			return nil, api.ErrStreamUnauthorized
		}
		var err error
		tor, err = torrents.Add(spec, meta.Title, meta.Poster, meta.Data, meta.Category)
		if err != nil {
			log.TLogln("[DEBUG] EnsureTorrent: Add error:", err)
			return nil, err
		}
		log.TLogln("[DEBUG] EnsureTorrent: Add succeeded, tor=", tor)
	}

	log.TLogln("[DEBUG] EnsureTorrent: calling GotInfo")
	if !tor.GotInfo() {
		log.TLogln("[DEBUG] EnsureTorrent: no GotInfo, returning connection timeout")
		return nil, api.ErrStreamConnectionTimeout
	}
	log.TLogln("[DEBUG] EnsureTorrent: GotInfo succeeded")
	if tor.Title == "" {
		tor.Title = tor.Name()
	}

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
