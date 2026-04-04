package api

import (
	"net/http"
	"server/torrshash"
	"strings"

	"server/log"
	"server/web/api/utils"

	"github.com/anacrolix/torrent"
	"github.com/gin-gonic/gin"
)

// Action: add, get, set, rem, list, drop
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
//	@Param			request	body	torrReqJS	true	"Torrent request. Available params for action: add, get, set, rem, list, drop, wipe. link required for add, hash required for get, set, rem, drop."
//
//	@Accept			json
//	@Produce		json
//	@Success		200
//	@Router			/torrents [post]
func torrents(c *gin.Context) {
	svc := getServices()
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

func addTorrent(svc *APIServices, req torrReqJS, c *gin.Context) {
	if req.Link == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("link", "is required for action=add"))
		return
	}

	log.TLogln("add torrent", req.Link)
	req.Link = strings.ReplaceAll(req.Link, "&amp;", "&")

	var torrSpec *torrent.TorrentSpec
	var torrsHash *torrshash.TorrsHash
	var err error

	if strings.HasPrefix(req.Link, "torrs://") {
		torrSpec, torrsHash, err = utils.ParseTorrsHash(req.Link)
		if err != nil {
			log.TLogln("error parse torrshash:", err)
			abortAPIError(c, http.StatusBadRequest, newValidationError("link", "invalid torrs hash"))
			return
		}
		if req.Title == "" {
			req.Title = torrsHash.Title()
		}
		if req.Poster == "" {
			req.Poster = torrsHash.Poster()
		}
		if req.Category == "" {
			req.Category = torrsHash.Category()
		}
	} else {
		torrSpec, err = utils.ParseLink(req.Link)
		if err != nil {
			log.TLogln("error parse link:", err)
			abortAPIError(c, http.StatusBadRequest, newValidationError("link", "invalid magnet/hash/link"))
			return
		}
	}

	tor, err := svc.Torrents.Add(torrSpec, req.Title, req.Poster, req.Data, req.Category)
	if err != nil {
		log.TLogln("error add torrent:", err)
		abortAPIError(c, http.StatusInternalServerError, newInternalError("failed to add torrent", err))
		return
	}

	_ = svc.Torrents.EnqueueMetadataFinalize(tor, torrSpec, req.SaveToDB)

	if svc.Settings.EnableDLNA() {
		modulesErr := svc.Modules.RestartDLNA(true)
		if modulesErr != nil {
			log.TLogln("dlna restart error:", modulesErr)
		}
	}
	c.JSON(200, tor.Status())
}

func getTorrent(svc *APIServices, req torrReqJS, c *gin.Context) {
	if req.Hash == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("hash", "is required for action=get"))
		return
	}
	tor := svc.Torrents.Get(req.Hash)

	if tor != nil {
		st := tor.Status()
		c.JSON(200, st)
	} else {
		abortAPIError(c, http.StatusNotFound, newNotFoundError("torrent not found"))
	}
}

func setTorrent(svc *APIServices, req torrReqJS, c *gin.Context) {
	if req.Hash == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("hash", "is required for action=set"))
		return
	}
	svc.Torrents.Set(req.Hash, req.Title, req.Poster, req.Category, req.Data)
	c.Status(200)
}

func remTorrent(svc *APIServices, req torrReqJS, c *gin.Context) {
	if req.Hash == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("hash", "is required for action=rem"))
		return
	}
	svc.Torrents.Remove(req.Hash)
	// TODO: remove
	if svc.Settings.EnableDLNA() {
		if err := svc.Modules.RestartDLNA(true); err != nil {
			log.TLogln("dlna restart error:", err)
		}
	}
	c.Status(200)
}

func listTorrents(svc *APIServices, c *gin.Context) {
	c.JSON(200, listTorrentStatuses(svc.Torrents))
}

func dropTorrent(svc *APIServices, req torrReqJS, c *gin.Context) {
	if req.Hash == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("hash", "is required for action=drop"))
		return
	}
	svc.Torrents.Drop(req.Hash)
	c.Status(200)
}

func wipeTorrents(svc *APIServices, c *gin.Context) {
	torrents := svc.Torrents.List()
	for _, t := range torrents {
		svc.Torrents.Remove(t.TorrentSpec.InfoHash.HexString())
	}
	// TODO: remove (copied todo from remTorrent())
	if svc.Settings.EnableDLNA() {
		if err := svc.Modules.RestartDLNA(true); err != nil {
			log.TLogln("dlna restart error:", err)
		}
	}
	c.Status(200)
}
