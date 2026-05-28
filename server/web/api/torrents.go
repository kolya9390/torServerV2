package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"server/internal/app/contracts"
	"server/log"

	"github.com/gin-gonic/gin"
)

// torrents godoc
//
//	@Summary		Handle torrents informations
//	@Description	Allow to list, add, remove, get, set, drop, wipe torrents on server. The action depends of what has been asked.
//
//	@Tags			API
//
//	@Param			request	body	torrReqJS	true	"Torrent request. Available params: add, get, set, rem, list, drop, wipe."
//
//	@Accept			json
//	@Produce		json
//	@Success		200
//	@Router			/torrents [post]
func torrents(c *gin.Context) {
	req, err := bindTorrentRequest(c)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	deps, ok := torrentDepsFromContext(c)
	if !ok {
		return
	}

	logTorrentsActionRequest(c, req)

	switch req.Action {
	case "add":
		addTorrent(deps, req, c)
	case "get":
		getTorrent(deps, req, c)
	case "set":
		setTorrent(deps, req, c)
	case "rem":
		remTorrent(deps, req, c)
	case "list":
		listTorrents(deps, c)
	case "drop":
		dropTorrent(deps, req, c)
	case "wipe":
		wipeTorrents(deps, c)
	default:
		abortAPIError(c, http.StatusBadRequest, newValidationError("action", "must be one of: add, get, set, rem, list, drop, wipe"))
	}
}

func logTorrentsActionRequest(c *gin.Context, req torrReqJS) {
	if !log.IsDebugEnabled() {
		return
	}

	const maxLinkLogLen = 120

	link := req.Link
	if len(link) > maxLinkLogLen {
		link = link[:maxLinkLogLen] + "..."
	}

	log.DebugSampled(
		"api.torrents.action."+req.Action,
		100,
		"api torrents action",
		"action", req.Action,
		"hash", req.Hash,
		"save_to_db", req.SaveToDB,
		"ip", c.ClientIP(),
		"request_id", c.GetString("request_id"),
		"link", link,
	)
}

func addTorrent(deps torrentHandlerDeps, req torrReqJS, c *gin.Context) {
	if req.Link == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("link", "is required for action=add"))

		return
	}

	log.Debug("addTorrent: parse link", "link", req.Link)
	req.Link = strings.ReplaceAll(req.Link, "&amp;", "&")

	torrSpec, meta, err := deps.Parser.ParseLink(req.Link, req.Title, req.Poster, req.Category)
	if err != nil {
		log.TLogln("error parse torrent link:", err)
		abortAPIError(c, http.StatusBadRequest, torrentLinkValidationError(err))

		return
	}

	req.Title = meta.Title
	req.Poster = meta.Poster
	req.Category = meta.Category

	hashHex := torrSpec.HashHex()
	// Fast path for chatty clients: if torrent is already active in memory,
	// don't call Add again (can block under heavy concurrent stream load).
	log.Debug("addTorrent: before Torrents.Get", "hash", hashHex)
	existing := deps.Queries.Get(hashHex)
	log.Debug("addTorrent: after Torrents.Get", "hash", hashHex, "torrent_exists", existing != nil)

	if existing != nil && !deps.Queries.IsStored(existing) {
		if req.SaveToDB {
			log.Debug("addTorrent: enqueue save_to_db finalize", "hash", hashHex)

			_ = deps.Commands.EnqueueMetadataFinalize(existing, nil, true)
		}

		log.Debug("addTorrent: returning fast-path status", "hash", hashHex)
		writeTorrentStatusResponse(c, deps.Queries.Status(existing))

		return
	}

	log.Debug("addTorrent: calling Torrents.Add", "hash", hashHex)

	tor, err := deps.Commands.Add(torrSpec, req.Title, req.Poster, req.Data, req.Category)
	if err != nil {
		log.TLogln("error add torrent:", err)
		abortAPIError(c, http.StatusInternalServerError, newInternalError("failed to add torrent", err))

		return
	}

	log.Debug("addTorrent: Torrents.Add succeeded", "hash", hashHex)

	_ = deps.Commands.EnqueueMetadataFinalize(tor, &torrSpec, req.SaveToDB)

	if deps.Settings.EnableDLNA() {
		modulesErr := deps.Modules.RestartDLNA(true)
		if modulesErr != nil {
			log.TLogln("dlna restart error:", modulesErr)
		}
	}

	writeTorrentStatusResponse(c, deps.Queries.Status(tor))
}

func torrentLinkValidationError(err error) error {
	switch {
	case errors.Is(err, contracts.ErrStreamLinkEmpty):
		return newValidationError("link", "is required for action=add")
	case errors.Is(err, contracts.ErrStreamInvalidTorrsHash):
		return newValidationError("link", "invalid torrs hash")
	default:
		return newValidationError("link", "invalid magnet/hash/link")
	}
}

func getTorrent(deps torrentHandlerDeps, req torrReqJS, c *gin.Context) {
	if req.Hash == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("hash", "is required for action=get"))

		return
	}

	log.Debug("getTorrent: before Torrents.StatusByHash", "hash", req.Hash)
	st, found := deps.Queries.StatusByHash(req.Hash)
	log.Debug("getTorrent: after Torrents.StatusByHash", "hash", req.Hash, "found", found)

	if found {
		log.Debug("getTorrent: using status", "hash", req.Hash)
		writeTorrentStatusResponse(c, st)
	} else {
		abortAPIError(c, http.StatusNotFound, newNotFoundError("torrent not found"))
	}
}

func setTorrent(deps torrentHandlerDeps, req torrReqJS, c *gin.Context) {
	if req.Hash == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("hash", "is required for action=set"))

		return
	}

	deps.Commands.Set(req.Hash, req.Title, req.Poster, req.Category, req.Data)
	c.Status(200)
}

func remTorrent(deps torrentHandlerDeps, req torrReqJS, c *gin.Context) {
	if req.Hash == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("hash", "is required for action=rem"))

		return
	}

	deps.Commands.Remove(req.Hash)
	// Restart DLNA to reflect updated torrent list
	if deps.Settings.EnableDLNA() {
		if err := deps.Modules.RestartDLNA(true); err != nil {
			log.TLogln("dlna restart error:", err)
		}
	}

	c.Status(200)
}

func listTorrents(deps torrentHandlerDeps, c *gin.Context) {
	writeTorrentStatusListResponse(c, deps.Queries.Statuses())
}

func dropTorrent(deps torrentHandlerDeps, req torrReqJS, c *gin.Context) {
	if req.Hash == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("hash", "is required for action=drop"))

		return
	}

	readiness := deps.Commands.DropReadiness(req.Hash)
	if readiness.ActiveReaders > 0 {
		log.TLogln("drop skipped: active readers", "hash=", req.Hash, "readers=", readiness.ActiveReaders)
		abortAPIError(c, http.StatusConflict, newConflictError("torrent has active streams"))

		return
	}

	// Active stream count is global and catches long-lived responses where
	// reader count can transiently be zero between range reconnects.
	if readiness.ActiveStreams > 0 {
		log.TLogln("drop skipped: active stream sessions", "hash=", req.Hash, "active_streams=", readiness.ActiveStreams)
		abortAPIError(c, http.StatusConflict, newConflictError("stream session is active"))

		return
	}

	// Protect playback against short reconnect gaps where active readers can momentarily drop to zero.
	if readiness.RecentStreamElapsed < 5*time.Second {
		log.TLogln("drop skipped: recent stream activity", "hash=", req.Hash)
		abortAPIError(c, http.StatusConflict, newConflictError("stream reconnect in progress"))

		return
	}

	deps.Commands.Drop(req.Hash)
	c.Status(200)
}

func wipeTorrents(deps torrentHandlerDeps, c *gin.Context) {
	for _, hash := range deps.Queries.ListHashes() {
		deps.Commands.Remove(hash)
	}
	// Restart DLNA to reflect updated torrent list
	if deps.Settings.EnableDLNA() {
		if err := deps.Modules.RestartDLNA(true); err != nil {
			log.TLogln("dlna restart error:", err)
		}
	}

	c.Status(200)
}
