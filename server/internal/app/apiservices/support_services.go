package apiservices

import (
	"encoding/json"
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

func (d viewedService) SetViewed(v *contracts.ViewedItem) {
	d.setViewed(mapViewedItemToSettings(v))
}

func (d viewedService) RemoveViewed(v *contracts.ViewedItem) {
	d.removeViewed(mapViewedItemToSettings(v))
}

func (d viewedService) ListViewed(hash string) []*contracts.ViewedItem {
	log.Debug("viewedService.ListViewed: calling backend", "hash", hash)
	result := mapViewedItemsFromSettings(d.listViewed(hash))
	log.Debug("viewedService.ListViewed: got result", "items", len(result))

	return result
}

func mapViewedItemToSettings(item *contracts.ViewedItem) *sets.Viewed {
	if item == nil {
		return nil
	}

	return &sets.Viewed{
		Hash:      item.Hash,
		FileIndex: item.FileIndex,
	}
}

func mapViewedItemsFromSettings(items []*sets.Viewed) []*contracts.ViewedItem {
	if len(items) == 0 {
		return nil
	}

	mapped := make([]*contracts.ViewedItem, 0, len(items))

	for _, item := range items {
		if item == nil {
			continue
		}

		mapped = append(mapped, &contracts.ViewedItem{
			Hash:      item.Hash,
			FileIndex: item.FileIndex,
		})
	}

	return mapped
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

func (d mediaService) ProbePlayURL(hash, fileID string) (contracts.MediaProbe, error) {
	serverCfg := d.runtimeState().ServerConfig()
	link := fmt.Sprintf("http://127.0.0.1:%s/play/%s/%s", serverCfg.Port, hash, fileID)

	if serverCfg.SSL {
		link = fmt.Sprintf("https://127.0.0.1:%s/play/%s/%s", serverCfg.SSLPort, hash, fileID)
	}

	probe, err := ffprobe.ProbeURL(link)
	if err != nil {
		return nil, err
	}

	return mapMediaProbe(probe)
}

func mapMediaProbe(probe *goffprobe.ProbeData) (contracts.MediaProbe, error) {
	if probe == nil {
		return nil, nil
	}

	raw, err := json.Marshal(probe)
	if err != nil {
		return nil, fmt.Errorf("marshal media probe: %w", err)
	}

	var mapped contracts.MediaProbe
	if err := json.Unmarshal(raw, &mapped); err != nil {
		return nil, fmt.Errorf("unmarshal media probe: %w", err)
	}

	return mapped, nil
}

func (d modulesService) RestartDLNA(enable bool) error {
	return modules.RestartDLNAWithProviders(enable, d.provider, d.argsProvider)
}

func (modulesService) StopDLNA() {
	modules.StopDLNA()
}
