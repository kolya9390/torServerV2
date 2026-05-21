package apiservices

import (
	"fmt"

	goffprobe "gopkg.in/vansante/go-ffprobe.v2"

	"server/ffprobe"
	"server/log"
	"server/modules"
	sets "server/settings"
	"server/torznab"
)

func (d systemService) Shutdown() {
	if d.runtimeController != nil {
		d.runtimeController.Shutdown()
	}
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
