package api

import (
	"path/filepath"
	"sort"
	"strings"

	"server/torr/state"
)

func findFileNamesakes(files []*state.TorrentFileStat, file *state.TorrentFileStat) []*state.TorrentFileStat {
	name := filepath.Base(strings.TrimSuffix(file.Path, filepath.Ext(file.Path)))

	var namesakes []*state.TorrentFileStat

	for _, f := range files {
		if f != file && strings.Contains(f.Path, name) {
			namesakes = append(namesakes, f)
		}
	}

	return namesakes
}

func searchLastPlayed(viewedSvc ViewedService, tor *state.TorrentStatus) int {
	viewed := viewedSvc.ListViewed(tor.Hash)
	if len(viewed) == 0 {
		return -1
	}

	sort.Slice(viewed, func(i, j int) bool {
		return viewed[i].FileIndex > viewed[j].FileIndex
	})

	lastViewedIndex := viewed[0].FileIndex
	for i, stat := range tor.FileStats {
		if stat.Id == lastViewedIndex {
			if i >= len(tor.FileStats) {
				return -1
			}

			return i
		}
	}

	return -1
}
