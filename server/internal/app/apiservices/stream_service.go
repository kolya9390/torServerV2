package apiservices

import (
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/anacrolix/torrent"

	"server/internal/app/contracts"
	"server/internal/torrentparse"
	"server/log"
	"server/torr/state"
	"server/torrshash"
)

func (d streamService) ParseLink(link, title, poster, category string) (contracts.TorrentSpec, contracts.StreamMeta, error) {
	if link == "" {
		return contracts.TorrentSpec{}, contracts.StreamMeta{}, contracts.ErrStreamLinkEmpty
	}

	meta := contracts.StreamMeta{
		Title:    title,
		Poster:   poster,
		Category: category,
	}

	var err error

	link, err = url.QueryUnescape(link)
	if err != nil {
		return contracts.TorrentSpec{}, meta, err
	}

	meta.Title, err = url.QueryUnescape(meta.Title)
	if err != nil {
		return contracts.TorrentSpec{}, meta, err
	}

	meta.Poster, err = url.QueryUnescape(meta.Poster)
	if err != nil {
		return contracts.TorrentSpec{}, meta, err
	}

	meta.Category, err = url.QueryUnescape(meta.Category)
	if err != nil {
		return contracts.TorrentSpec{}, meta, err
	}

	var spec *torrent.TorrentSpec

	if strings.HasPrefix(link, "torrs://") || (len(link) > 45 && torrshash.IsBase62(link)) {
		var torrsHash *torrshash.TorrsHash

		spec, torrsHash, err = torrentparse.ParseTorrsHash(link)
		if err != nil {
			return contracts.TorrentSpec{}, contracts.StreamMeta{}, contracts.ErrStreamInvalidTorrsHash
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

		return wrapTorrentSpec(spec), meta, nil
	}

	spec, err = torrentparse.ParseLink(link)
	if err != nil {
		return contracts.TorrentSpec{}, contracts.StreamMeta{}, contracts.ErrStreamInvalidLink
	}

	return wrapTorrentSpec(spec), meta, nil
}

func (d streamService) ParseTorrentFile(reader io.Reader) (contracts.TorrentSpec, error) {
	spec, err := torrentparse.ParseReader(reader)
	if err != nil {
		return contracts.TorrentSpec{}, err
	}

	return wrapTorrentSpec(spec), nil
}

func (d streamService) EnsureTorrent(torrents contracts.TorrentService, spec contracts.TorrentSpec, meta contracts.StreamMeta, allowCreate bool) (contracts.TorrentHandle, error) {
	log.Debug("EnsureTorrent: starting", "hash", spec.HashHex())
	log.Debug("EnsureTorrent: about to call torrents.Get")

	tor := torrents.Get(spec.HashHex())
	log.Debug("EnsureTorrent: torrents.Get returned", "tor", tor != nil)

	if tor != nil {
		torStat := tor.State()
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
				return nil, contracts.ErrStreamUnauthorized
			}

			log.Debug("EnsureTorrent: activating torrent from DB metadata")

			tor = torrents.LoadFromDB(tor)
			if tor == nil {
				return nil, contracts.ErrStreamConnectionTimeout
			}

			if tor.State() == state.TorrentWorking || tor.State() == state.TorrentPreload {
				tor.EnsureTitleFromInfo()

				return tor, nil
			}
		}
	}

	if tor == nil {
		log.Debug("EnsureTorrent: need to add torrent")

		if !allowCreate {
			return nil, contracts.ErrStreamUnauthorized
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

	if !tor.Ready() {
		log.Debug("EnsureTorrent: no GotInfo, returning connection timeout")

		return nil, contracts.ErrStreamConnectionTimeout
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
		return 0, contracts.ErrStreamFileIndexInvalid
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
