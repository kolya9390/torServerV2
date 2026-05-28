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
	"server/torrshash"
)

func (streamService) ParseLink(link, title, poster, category string) (contracts.TorrentSpec, contracts.StreamMeta, error) {
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

func (streamService) ParseTorrentFile(reader io.Reader) (contracts.TorrentSpec, error) {
	spec, err := torrentparse.ParseReader(reader)
	if err != nil {
		return contracts.TorrentSpec{}, err
	}

	return wrapTorrentSpec(spec), nil
}

func (streamService) EnsureTorrent(
	torrents contracts.TorrentStreamService,
	spec contracts.TorrentSpec,
	meta contracts.StreamMeta,
	allowCreate bool,
) (contracts.TorrentHandle, error) {
	hash := spec.HashHex()
	tor := torrents.Get(hash)

	if tor != nil {
		torStat := tor.State()

		if isActiveStreamTorrent(torStat) {
			log.DebugSampled(
				"stream.ensure.active",
				100,
				"stream ensure active torrent fast path",
				"hash", hash,
				"state", torStat,
			)
			tor.EnsureTitleFromInfo()

			return tor, nil
		}

		if torStat == contracts.TorrentInDB {
			if !allowCreate {
				return nil, contracts.ErrStreamUnauthorized
			}

			log.Debug("stream ensure activating torrent from db", "hash", hash)

			tor = torrents.LoadFromDB(tor)
			if tor == nil {
				return nil, contracts.ErrStreamConnectionTimeout
			}

			if isActiveStreamTorrent(tor.State()) {
				tor.EnsureTitleFromInfo()

				return tor, nil
			}
		}
	}

	if tor == nil {
		log.Debug("stream ensure add torrent", "hash", hash)

		if !allowCreate {
			return nil, contracts.ErrStreamUnauthorized
		}

		var err error

		tor, err = torrents.Add(spec, meta.Title, meta.Poster, meta.Data, meta.Category)
		if err != nil {
			log.Debug("stream ensure add failed", "hash", hash, "error", err)

			return nil, err
		}
	}

	log.Debug("stream ensure waiting for torrent info", "hash", hash)

	if !tor.Ready() {
		log.Debug("stream ensure torrent info timeout", "hash", hash)

		return nil, contracts.ErrStreamConnectionTimeout
	}

	tor.EnsureTitleFromInfo()

	return tor, nil
}

func isActiveStreamTorrent(state contracts.TorrentState) bool {
	return state == contracts.TorrentWorking || state == contracts.TorrentPreload
}

func (streamService) ParseFileIndex(index string, fileCount int) (int, error) {
	if fileCount == 1 {
		return 1, nil
	}

	ind, err := strconv.Atoi(index)
	if err != nil || ind < 0 {
		return 0, contracts.ErrStreamFileIndexInvalid
	}

	return ind, nil
}

func (streamService) NormalizePlaylistName(rawName, fallback string) string {
	name := strings.ReplaceAll(rawName, `/`, "")
	if name == "" {
		return fallback + ".m3u"
	}

	if !strings.HasSuffix(strings.ToLower(name), ".m3u") && !strings.HasSuffix(strings.ToLower(name), ".m3u8") {
		return name + ".m3u"
	}

	return name
}
