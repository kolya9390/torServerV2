package apiservices

import (
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"server/internal/app/contracts"
	"server/internal/torrentparse"
	"server/utils"
)

func (d playbackService) BuildAllPlaylist(host string, torrents contracts.TorrentPlaylistService) contracts.PlaylistPayload {
	torrs := torrents.List()

	var body strings.Builder

	body.Grow(len(torrs) * 128)
	body.WriteString("#EXTM3U\n")

	var hash strings.Builder

	hash.Grow(len(torrs) * 40)

	// fn=file.m3u fixes forkplayer bug with trailing .m3u in link.
	for _, tr := range torrs {
		status := tr.Status()
		if status == nil {
			continue
		}

		body.WriteString("#EXTINF:0")

		if status.Poster != "" {
			body.WriteString(` tvg-logo="`)
			body.WriteString(status.Poster)
			body.WriteString(`"`)
		}

		body.WriteString(` type="playlist",`)
		body.WriteString(status.Title)
		body.WriteString("\n")
		body.WriteString(host)
		body.WriteString("/stream/")
		body.WriteString(url.PathEscape(status.Title))
		body.WriteString(".m3u?link=")
		body.WriteString(tr.HashHex())
		body.WriteString("&m3u&fn=file.m3u\n")
		hash.WriteString(tr.HashHex())
	}

	return contracts.PlaylistPayload{
		Name: "all.m3u",
		Hash: hash.String(),
		Body: body.String(),
	}
}

func (d playbackService) BuildPlaylistByHash(hash, requestedName string, fromLast bool,
	host string, torrents contracts.TorrentPlaylistService, viewed contracts.ViewedService) (contracts.PlaylistPayload, error) {
	if hash == "" {
		return contracts.PlaylistPayload{}, contracts.ErrPlaylistHashRequired
	}

	tor := torrents.Get(hash)
	if tor == nil {
		return contracts.PlaylistPayload{}, contracts.ErrPlaylistTorrentNotFound
	}

	if tor.State() == contracts.TorrentInDB {
		tor = torrents.LoadFromDB(tor)
		if tor == nil {
			return contracts.PlaylistPayload{}, contracts.ErrPlaylistLoadFailed
		}
	}

	name := normalizePlaylistName(requestedName, tor.Name())
	body := d.BuildM3UFromStatus(tor.Status(), host, fromLast, viewed)

	return contracts.PlaylistPayload{
		Name: name,
		Hash: tor.HashHex(),
		Body: body,
	}, nil
}

func (d playbackService) ResolvePlay(hash, index string, unauthorized bool, torrents contracts.TorrentPlayService) (contracts.PlayTarget, error) {
	if hash == "" || index == "" {
		return contracts.PlayTarget{}, contracts.ErrPlayPathRequired
	}

	spec, err := torrentparse.ParseLink(hash)
	if err != nil {
		return contracts.PlayTarget{}, contracts.ErrPlayHashInvalid
	}

	appSpec := wrapTorrentSpec(spec)

	tor := torrents.Get(appSpec.HashHex())
	if tor == nil && unauthorized {
		return contracts.PlayTarget{}, contracts.ErrPlayUnauthorized
	}

	if tor == nil {
		return contracts.PlayTarget{}, contracts.ErrPlayTorrentNotFound
	}

	if tor.State() == contracts.TorrentInDB {
		meta := tor.Metadata()

		tor, err = torrents.Add(appSpec, meta.Title, meta.Poster, meta.Data, meta.Category)
		if err != nil {
			return contracts.PlayTarget{}, fmt.Errorf("%w: %v", contracts.ErrPlayLoadFailed, err)
		}
	}

	if !tor.Ready() {
		return contracts.PlayTarget{}, contracts.ErrPlayTimeout
	}

	fileIndex := -1
	if tor.FileCount() == 1 {
		fileIndex = 1
	} else {
		ind, parseErr := strconv.Atoi(index)
		if parseErr == nil {
			fileIndex = ind
		}
	}

	if fileIndex == -1 {
		return contracts.PlayTarget{}, contracts.ErrPlayFileIndexInvalid
	}

	return contracts.PlayTarget{
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

func (d playbackService) BuildM3UFromStatus(tor *contracts.TorrentStatus, host string, fromLast bool, viewed contracts.ViewedService) string {
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
				body.WriteString(strconv.Itoa(namesake.ID))
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
		body.WriteString(strconv.Itoa(f.ID))
		body.WriteString("&play\n")
	}

	return body.String()
}

func findFileNamesakes(files []*contracts.TorrentFile, file *contracts.TorrentFile) []*contracts.TorrentFile {
	name := filepath.Base(strings.TrimSuffix(file.Path, filepath.Ext(file.Path)))

	var namesakes []*contracts.TorrentFile

	for _, f := range files {
		// External audio/subtitle files usually contain video filename fragment.
		if f != file && strings.Contains(f.Path, name) {
			namesakes = append(namesakes, f)
		}
	}

	return namesakes
}

func searchLastPlayed(viewedSvc contracts.ViewedService, tor *contracts.TorrentStatus) int {
	viewed := viewedSvc.ListViewed(tor.Hash)
	if len(viewed) == 0 {
		return -1
	}

	sort.Slice(viewed, func(i, j int) bool {
		return viewed[i].FileIndex > viewed[j].FileIndex
	})

	lastViewedIndex := viewed[0].FileIndex
	for i, stat := range tor.FileStats {
		if stat.ID == lastViewedIndex {
			if i >= len(tor.FileStats) {
				return -1
			}

			return i
		}
	}

	return -1
}
