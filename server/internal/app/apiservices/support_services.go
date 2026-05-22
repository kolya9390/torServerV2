package apiservices

import (
	"fmt"

	goffprobe "gopkg.in/vansante/go-ffprobe.v2"

	"server/ffprobe"
	"server/internal/app/contracts"
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

func (d searchService) TorznabSearch(query string, index int) []*contracts.SearchResult {
	return mapTorznabResults(torznab.SearchWithProvider(query, index, d.provider))
}

func (searchService) TorznabTest(host, key string) error {
	return torznab.Test(host, key)
}

func mapTorznabResults(results []*torznab.TorrentDetails) []*contracts.SearchResult {
	if len(results) == 0 {
		return nil
	}

	mapped := make([]*contracts.SearchResult, 0, len(results))

	for _, result := range results {
		if result == nil {
			continue
		}

		mapped = append(mapped, &contracts.SearchResult{
			Title:      result.Title,
			Name:       result.Name,
			Link:       result.Link,
			Magnet:     result.Magnet,
			Hash:       result.Hash,
			Size:       result.Size,
			Seed:       result.Seed,
			Peer:       result.Peer,
			CreateDate: result.CreateDate,
			Categories: result.Categories,
			Year:       result.Year,
		})
	}

	return mapped
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

func (modulesService) StopDLNA() {
	modules.StopDLNA()
}
