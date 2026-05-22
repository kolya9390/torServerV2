package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"server/internal/app/contracts"
	"server/log"
	utils2 "server/utils"
	"server/web/api/utils"
)

type legacyStreamAction int

const (
	legacyStreamActionNone legacyStreamAction = iota
	legacyStreamActionPreload
	legacyStreamActionStat
	legacyStreamActionM3U
	legacyStreamActionPlay
	legacyStreamActionSave
)

type legacyStreamAdapter struct {
	deps streamHandlerDeps
}

type legacyStreamRequest struct {
	link     string
	preload  bool
	stat     bool
	save     bool
	m3u      bool
	fromlast bool
	play     bool
	notAuth  bool
}

type legacyStreamTarget struct {
	tor           contracts.TorrentHandle
	index         int
	preloadQueued bool
}

func newLegacyStreamAdapter(deps streamHandlerDeps) legacyStreamAdapter {
	return legacyStreamAdapter{deps: deps}
}

func (a legacyStreamAdapter) Handle(c *gin.Context) {
	req := parseLegacyStreamRequest(c)

	if handleExplicitLegacyRoute(c, req) {
		return
	}

	a.handleCompatibility(c, req)
}

func parseLegacyStreamRequest(c *gin.Context) legacyStreamRequest {
	_, preload := c.GetQuery("preload")
	_, stat := c.GetQuery("stat")
	_, save := c.GetQuery("save")
	_, m3u := c.GetQuery("m3u")
	_, fromlast := c.GetQuery("fromlast")
	_, play := c.GetQuery("play")

	return legacyStreamRequest{
		link:     c.Query("link"),
		preload:  preload,
		stat:     stat,
		save:     save,
		m3u:      m3u,
		fromlast: fromlast,
		play:     play,
		notAuth:  isNotAuthRequest(c),
	}
}

func handleExplicitLegacyRoute(c *gin.Context, req legacyStreamRequest) bool {
	switch req.explicitAction() {
	case legacyStreamActionStat:
		streamStat(c)
	case legacyStreamActionM3U:
		streamM3U(c)
	case legacyStreamActionSave:
		streamSave(c)
	case legacyStreamActionPlay:
		streamPlay(c)
	default:
		return false
	}

	return true
}

func (req legacyStreamRequest) explicitAction() legacyStreamAction {
	switch {
	case req.stat && !req.play && !req.save && !req.m3u:
		return legacyStreamActionStat
	case req.m3u && !req.play && !req.save && !req.stat:
		return legacyStreamActionM3U
	case req.save && !req.play && !req.stat && !req.m3u:
		return legacyStreamActionSave
	case req.play && !req.stat && !req.m3u && !req.save:
		return legacyStreamActionPlay
	default:
		return legacyStreamActionNone
	}
}

func (a legacyStreamAdapter) handleCompatibility(c *gin.Context, req legacyStreamRequest) {
	if a.handleAuth(c, req) {
		return
	}

	target, ok := a.prepareTarget(c, true, req.play || req.preload)
	if !ok {
		return
	}

	// Legacy: save can be combined with play/m3u/stat/preload.
	if req.save {
		a.deps.Torrents.SaveToDB(target.tor)
	}

	if req.preload {
		target.preloadQueued = a.enqueuePreload(target.tor, target.index)
	}

	a.handleAction(c, req, target)
}

func (a legacyStreamAdapter) handleAuth(c *gin.Context, req legacyStreamRequest) bool {
	if !req.notAuth {
		return false
	}

	if err := utils.TestLink(req.link, !req.notAuth); err != nil {
		abortAPIError(c, http.StatusBadRequest, newValidationError("link", "wrong link"))

		return true
	}

	if req.play || req.m3u {
		a.handleNoAuth(c, req)

		return true
	}

	c.Header("WWW-Authenticate", "Basic realm=Authorization Required")
	abortAPIError(c, http.StatusUnauthorized, newUnauthorizedError("authorization required"))

	return true
}

func (a legacyStreamAdapter) prepareTarget(c *gin.Context, allowCreate, requireValidIndex bool) (legacyStreamTarget, bool) {
	if c.Query("link") == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("link", "should not be empty"))

		return legacyStreamTarget{}, false
	}

	spec, meta, err := parseStreamLink(c, a.deps.Parser)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return legacyStreamTarget{}, false
	}

	tor, err := a.deps.Streams.EnsureTorrent(a.deps.Torrents, spec, contracts.StreamMeta{
		Title:    meta.title,
		Poster:   meta.poster,
		Category: meta.category,
		Data:     meta.data,
	}, allowCreate)
	if err != nil {
		statusCode, apiErr := mapStreamEnsureError(err)
		if statusCode == http.StatusUnauthorized {
			c.Header("WWW-Authenticate", "Basic realm=Authorization Required")
		}

		abortAPIError(c, statusCode, apiErr)

		return legacyStreamTarget{}, false
	}

	index, err := parseStreamFileIndex(c, a.deps.Parser, tor.FileCount())
	if err != nil && requireValidIndex {
		abortAPIError(c, http.StatusBadRequest, err)

		return legacyStreamTarget{}, false
	}

	return legacyStreamTarget{tor: tor, index: index}, true
}

func (a legacyStreamAdapter) handleAction(c *gin.Context, req legacyStreamRequest, target legacyStreamTarget) {
	switch req.compatibilityAction() {
	case legacyStreamActionPreload:
		if !target.preloadQueued {
			abortAPIError(c, http.StatusServiceUnavailable, newInternalError("preload queue is full", nil))

			return
		}

		c.JSON(http.StatusAccepted, gin.H{"status": "preload accepted", "hash": target.tor.HashHex()})
	case legacyStreamActionStat:
		c.JSON(http.StatusOK, target.tor.Status())
	case legacyStreamActionM3U:
		a.sendM3U(c, target.tor, req.fromlast)
	case legacyStreamActionPlay:
		streamTorrentFile(c, target.tor, target.index)
	case legacyStreamActionSave:
		c.Status(http.StatusOK)
	default:
		abortAPIError(c, http.StatusBadRequest, newValidationError("action", "no supported stream action specified"))
	}
}

func (req legacyStreamRequest) compatibilityAction() legacyStreamAction {
	switch {
	case req.preload && !req.stat && !req.m3u && !req.play && !req.save:
		return legacyStreamActionPreload
	case req.stat:
		return legacyStreamActionStat
	case req.m3u:
		return legacyStreamActionM3U
	case req.play:
		return legacyStreamActionPlay
	case req.save:
		return legacyStreamActionSave
	default:
		return legacyStreamActionNone
	}
}

func (a legacyStreamAdapter) enqueuePreload(tor contracts.TorrentHandle, index int) bool {
	queued := a.deps.Torrents.EnqueuePreload(tor, index)
	if !queued {
		log.TLogln("preload queue is full, skipping preload")
	}

	return queued
}

func (a legacyStreamAdapter) sendM3U(c *gin.Context, tor contracts.TorrentHandle, fromlast bool) {
	name := a.deps.Helpers.NormalizePlaylistName(c.Param("fname"), tor.Name())
	host := utils2.GetScheme(c) + "://" + utils2.GetHost(c)
	m3ulist := a.deps.Playback.BuildM3UFromStatus(tor.Status(), host, fromlast, a.deps.Viewed)
	sendM3U(c, name, tor.HashHex(), m3ulist)
}

func (a legacyStreamAdapter) handleNoAuth(c *gin.Context, req legacyStreamRequest) {
	target, ok := a.prepareTarget(c, false, req.play)
	if !ok {
		return
	}

	if req.preload {
		target.preloadQueued = a.enqueuePreload(target.tor, target.index)
	}

	if req.m3u {
		a.sendM3U(c, target.tor, req.fromlast)

		return
	}

	if req.play {
		streamTorrentFile(c, target.tor, target.index)

		return
	}

	c.Header("WWW-Authenticate", "Basic realm=Authorization Required")
	abortAPIError(c, http.StatusUnauthorized, newUnauthorizedError("authorization required"))
}

func streamTorrentFile(c *gin.Context, tor contracts.TorrentHandle, index int) {
	if err := c.Request.Context().Err(); err != nil {
		abortAPIError(c, http.StatusRequestTimeout, newValidationError("request", "request canceled"))

		return
	}

	if err := tor.Stream(index, c.Request, c.Writer); err != nil {
		c.Error(err) //nolint:errcheck // gin adds error to context
	}
}
