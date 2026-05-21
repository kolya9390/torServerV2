package api

import (
	"errors"
	"net/http"
	"server/internal/app/contracts"
	"strings"
	"time"

	"server/log"

	"github.com/gin-gonic/gin"
)

// Action: add, get, set, rem, list, drop.
type torrReqJS struct {
	requestI
	Link     string `json:"link,omitempty"`
	Hash     string `json:"hash,omitempty"`
	Title    string `json:"title,omitempty"`
	Category string `json:"category,omitempty"`
	Poster   string `json:"poster,omitempty"`
	Data     string `json:"data,omitempty"`
	SaveToDB bool   `json:"save_to_db,omitempty"`
}

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
	svc := servicesFromContext(c)

	var req torrReqJS

	err := c.ShouldBindJSON(&req)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, newValidationError("request", "invalid json body"))

		return
	}

	if req.Action == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("action", "is required"))

		return
	}

	logTorrentsActionRequest(c, req)

	switch req.Action {
	case "add":
		addTorrent(svc, req, c)
	case "get":
		getTorrent(svc, req, c)
	case "set":
		setTorrent(svc, req, c)
	case "rem":
		remTorrent(svc, req, c)
	case "list":
		listTorrents(svc, c)
	case "drop":
		dropTorrent(svc, req, c)
	case "wipe":
		wipeTorrents(svc, c)
	default:
		abortAPIError(c, http.StatusBadRequest, newValidationError("action", "must be one of: add, get, set, rem, list, drop, wipe"))
	}
}

func logTorrentsActionRequest(c *gin.Context, req torrReqJS) {
	const maxLinkLogLen = 120

	link := req.Link
	if len(link) > maxLinkLogLen {
		link = link[:maxLinkLogLen] + "..."
	}

	log.TLogln(
		"[API /torrents] action=", req.Action,
		" hash=", req.Hash,
		" save_to_db=", req.SaveToDB,
		" ip=", c.ClientIP(),
		" request_id=", c.GetString("request_id"),
		" link=", link,
	)
}

func addTorrent(svc *contracts.APIServices, req torrReqJS, c *gin.Context) {
	if req.Link == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("link", "is required for action=add"))

		return
	}

	log.TLogln("add torrent", req.Link)
	req.Link = strings.ReplaceAll(req.Link, "&amp;", "&")

	torrSpec, meta, err := svc.Streams.ParseLink(req.Link, req.Title, req.Poster, req.Category)
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
	log.TLogln("[TRACE] addTorrent: before Torrents.Get, hash=", hashHex)
	existing := svc.Torrents.Get(hashHex)
	log.TLogln("[TRACE] addTorrent: after Torrents.Get, hash=", hashHex, " tor=", existing != nil)

	if existing != nil && !svc.Torrents.IsStored(existing) {
		if req.SaveToDB {
			log.TLogln("[TRACE] addTorrent: enqueue save_to_db finalize, hash=", hashHex)

			_ = svc.Torrents.EnqueueMetadataFinalize(existing, nil, true)
		}

		log.TLogln("[TRACE] addTorrent: returning fast-path status, hash=", hashHex)
		c.JSON(200, svc.Torrents.Status(existing))

		return
	}

	log.Debug("addTorrent: calling Torrents.Add", "hash", hashHex)

	tor, err := svc.Torrents.Add(torrSpec, req.Title, req.Poster, req.Data, req.Category)
	if err != nil {
		log.TLogln("error add torrent:", err)
		abortAPIError(c, http.StatusInternalServerError, newInternalError("failed to add torrent", err))

		return
	}

	log.Debug("addTorrent: Torrents.Add succeeded", "hash", hashHex)

	_ = svc.Torrents.EnqueueMetadataFinalize(tor, &torrSpec, req.SaveToDB)

	if svc.Settings.EnableDLNA() {
		modulesErr := svc.Modules.RestartDLNA(true)
		if modulesErr != nil {
			log.TLogln("dlna restart error:", modulesErr)
		}
	}

	c.JSON(200, svc.Torrents.Status(tor))
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

func getTorrent(svc *contracts.APIServices, req torrReqJS, c *gin.Context) {
	if req.Hash == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("hash", "is required for action=get"))

		return
	}

	log.TLogln("[TRACE] getTorrent: before Torrents.StatusByHash, hash=", req.Hash)
	st, found := svc.Torrents.StatusByHash(req.Hash)
	log.TLogln("[TRACE] getTorrent: after Torrents.StatusByHash, hash=", req.Hash, " found=", found)

	if found {
		log.TLogln("[TRACE] getTorrent: using status, hash=", req.Hash)
		c.JSON(200, st)
	} else {
		abortAPIError(c, http.StatusNotFound, newNotFoundError("torrent not found"))
	}
}

func setTorrent(svc *contracts.APIServices, req torrReqJS, c *gin.Context) {
	if req.Hash == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("hash", "is required for action=set"))

		return
	}

	svc.Torrents.Set(req.Hash, req.Title, req.Poster, req.Category, req.Data)
	c.Status(200)
}

func remTorrent(svc *contracts.APIServices, req torrReqJS, c *gin.Context) {
	if req.Hash == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("hash", "is required for action=rem"))

		return
	}

	svc.Torrents.Remove(req.Hash)
	// Restart DLNA to reflect updated torrent list
	if svc.Settings.EnableDLNA() {
		if err := svc.Modules.RestartDLNA(true); err != nil {
			log.TLogln("dlna restart error:", err)
		}
	}

	c.Status(200)
}

func listTorrents(svc *contracts.APIServices, c *gin.Context) {
	c.JSON(200, svc.Torrents.Statuses())
}

func dropTorrent(svc *contracts.APIServices, req torrReqJS, c *gin.Context) {
	if req.Hash == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("hash", "is required for action=drop"))

		return
	}

	readiness := svc.Torrents.DropReadiness(req.Hash)
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

	svc.Torrents.Drop(req.Hash)
	c.Status(200)
}

func wipeTorrents(svc *contracts.APIServices, c *gin.Context) {
	for _, hash := range svc.Torrents.ListHashes() {
		svc.Torrents.Remove(hash)
	}
	// Restart DLNA to reflect updated torrent list
	if svc.Settings.EnableDLNA() {
		if err := svc.Modules.RestartDLNA(true); err != nil {
			log.TLogln("dlna restart error:", err)
		}
	}

	c.Status(200)
}
