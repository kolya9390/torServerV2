package apiservices

import (
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"server/internal/torrentparse"
	"server/torr/state"
	"server/utils"
	"server/web/api"
)

func (d playbackService) BuildAllPlaylist(host string, torrents api.TorrentService) api.PlaylistPayload {
	torrs := torrents.List()
	var body strings.Builder
	body.Grow(len(torrs) * 128)
	body.WriteString("#EXTM3U\n")

	var hash strings.Builder
	hash.Grow(len(torrs) * 40)

	// fn=file.m3u fixes forkplayer bug with trailing .m3u in link.
	for _, tr := range torrs {
		body.WriteString("#EXTINF:0")
		if tr.Poster != "" {
			body.WriteString(` tvg-logo="`)
			body.WriteString(tr.Poster)
			body.WriteString(`"`)
		}
		body.WriteString(` type="playlist",`)
		body.WriteString(tr.Title)
		body.WriteString("\n")
		body.WriteString(host)
		body.WriteString("/stream/")
		body.WriteString(url.PathEscape(tr.Title))
		body.WriteString(".m3u?link=")
		body.WriteString(tr.TorrentSpec.InfoHash.HexString())
		body.WriteString("&m3u&fn=file.m3u\n")
		hash.WriteString(tr.Hash().HexString())
	}

	return api.PlaylistPayload{
		Name: "all.m3u",
		Hash: hash.String(),
		Body: body.String(),
	}
}

func (d playbackService) BuildPlaylistByHash(hash, requestedName string, fromLast bool, host string, torrents api.TorrentService, viewed api.ViewedService) (api.PlaylistPayload, error) {
	if hash == "" {
		return api.PlaylistPayload{}, api.ErrPlaylistHashRequired
	}

	tor := torrents.Get(hash)
	if tor == nil {
		return api.PlaylistPayload{}, api.ErrPlaylistTorrentNotFound
	}

	if tor.Stat == state.TorrentInDB {
		tor = torrents.LoadFromDB(tor)
		if tor == nil {
			return api.PlaylistPayload{}, api.ErrPlaylistLoadFailed
		}
	}

	name := normalizePlaylistName(requestedName, tor.Name())
	body := d.BuildM3UFromStatus(tor.Status(), host, fromLast, viewed)
	return api.PlaylistPayload{
		Name: name,
		Hash: tor.Hash().HexString(),
		Body: body,
	}, nil
}

func (d playbackService) ResolvePlay(hash, index string, unauthorized bool, torrents api.TorrentService) (api.PlayTarget, error) {
	if hash == "" || index == "" {
		return api.PlayTarget{}, api.ErrPlayPathRequired
	}

	spec, err := torrentparse.ParseLink(hash)
	if err != nil {
		return api.PlayTarget{}, api.ErrPlayHashInvalid
	}

	tor := torrents.Get(spec.InfoHash.HexString())
	if tor == nil && unauthorized {
		return api.PlayTarget{}, api.ErrPlayUnauthorized
	}
	if tor == nil {
		return api.PlayTarget{}, api.ErrPlayTorrentNotFound
	}

	if tor.Stat == state.TorrentInDB {
		tor, err = torrents.Add(spec, tor.Title, tor.Poster, tor.Data, tor.Category)
		if err != nil {
			return api.PlayTarget{}, fmt.Errorf("%w: %v", api.ErrPlayLoadFailed, err)
		}
	}

	if !tor.GotInfo() {
		return api.PlayTarget{}, api.ErrPlayTimeout
	}

	fileIndex := -1
	if len(tor.Files()) == 1 {
		fileIndex = 1
	} else {
		ind, parseErr := strconv.Atoi(index)
		if parseErr == nil {
			fileIndex = ind
		}
	}
	if fileIndex == -1 {
		return api.PlayTarget{}, api.ErrPlayFileIndexInvalid
	}

	return api.PlayTarget{
		Torrent:   tor,
		FileIndex: fileIndex,
	}, nil
}

func normalizePlaylistName(rawName, fallback string) string {
	name := strings.TrimPrefix(rawName, "/")
	if name == "" {
		return fallback + ".m3u"
	}
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".m3u") || strings.HasSuffix(lower, ".m3u8") {
		return name
	}
	return name + ".m3u"
}

func (d playbackService) BuildM3UFromStatus(tor *state.TorrentStatus, host string, fromLast bool, viewed api.ViewedService) string {
	var body strings.Builder
	from := 0
	if fromLast {
		pos := searchLastPlayed(viewed, tor)
		if pos != -1 {
			from = pos
		}
	}

	hasPlayableFiles := false
	for i, f := range tor.FileStats {
		if i >= from && utils.GetMimeType(f.Path) != "*/*" {
			hasPlayableFiles = true
			break
		}
	}

	if !hasPlayableFiles {
		return ""
	}

	body.WriteString("#EXTM3U\n")

	for i, f := range tor.FileStats {
		if i < from || utils.GetMimeType(f.Path) == "*/*" {
			continue
		}

		fn := filepath.Base(f.Path)
		if fn == "" {
			fn = f.Path
		}

		body.WriteString("#EXTINF:0,")
		body.WriteString(fn)
		body.WriteString("\n")

		fileNamesakes := findFileNamesakes(tor.FileStats, f)
		if len(fileNamesakes) > 0 {
			body.WriteString("#EXTVLCOPT:input-slave=")
			for _, namesake := range fileNamesakes {
				sname := filepath.Base(namesake.Path)
				body.WriteString(host)
				body.WriteString("/stream/")
				body.WriteString(url.PathEscape(sname))
				body.WriteString("?link=")
				body.WriteString(tor.Hash)
				body.WriteString("&index=")
				body.WriteString(strconv.Itoa(namesake.Id))
				body.WriteString("&play#")
			}
			body.WriteString("\n")
		}

		name := filepath.Base(f.Path)
		body.WriteString(host)
		body.WriteString("/stream/")
		body.WriteString(url.PathEscape(name))
		body.WriteString("?link=")
		body.WriteString(tor.Hash)
		body.WriteString("&index=")
		body.WriteString(strconv.Itoa(f.Id))
		body.WriteString("&play\n")
	}
	return body.String()
}

func findFileNamesakes(files []*state.TorrentFileStat, file *state.TorrentFileStat) []*state.TorrentFileStat {
	name := filepath.Base(strings.TrimSuffix(file.Path, filepath.Ext(file.Path)))
	var namesakes []*state.TorrentFileStat
	for _, f := range files {
		// External audio/subtitle files usually contain video filename fragment.
		if f != file && strings.Contains(f.Path, name) {
			namesakes = append(namesakes, f)
		}
	}
	return namesakes
}

func searchLastPlayed(viewedSvc api.ViewedService, tor *state.TorrentStatus) int {
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
