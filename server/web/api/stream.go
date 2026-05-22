package api

import (
	"net/http"
	"server/internal/app/contracts"

	"server/log"
	utils2 "server/utils"
	"server/web/api/utils"

	"github.com/gin-gonic/gin"
)

// streamFlags holds the boolean query flags parsed from a stream request.
type streamFlags struct {
	preload, stat, save, m3u, fromlast, play bool
}

type legacyStreamTarget struct {
	tor           contracts.TorrentHandle
	index         int
	preloadQueued bool
}

// validateStreamRequest extracts boolean query flags from the stream request.
func validateStreamRequest(c *gin.Context) streamFlags {
	_, preload := c.GetQuery("preload")
	_, stat := c.GetQuery("stat")
	_, save := c.GetQuery("save")
	_, m3u := c.GetQuery("m3u")
	_, fromlast := c.GetQuery("fromlast")
	_, play := c.GetQuery("play")

	return streamFlags{preload, stat, save, m3u, fromlast, play}
}

// handleStreamAuth validates authentication for stream requests.
// Returns true if the handler should return early (response already written).
func handleStreamAuth(c *gin.Context, link string, notAuth, play, m3u bool) bool {
	if !notAuth {
		return false
	}

	if err := utils.TestLink(link, !notAuth); err != nil {
		abortAPIError(c, http.StatusBadRequest, newValidationError("link", "wrong link"))

		return true
	}

	if play || m3u {
		streamNoAuth(c)

		return true
	}

	c.Header("WWW-Authenticate", "Basic realm=Authorization Required")
	abortAPIError(c, http.StatusUnauthorized, newUnauthorizedError("authorization required"))

	return true
}

// stream godoc
//
//	@Summary		Multi usage endpoint
//	@Description	Multi usage endpoint.
//
//	@Tags			API
//
//	@Param			link		query	string	true	"Magnet/hash/link to torrent"
//	@Param			index		query	string	false	"File index in torrent"
//	@Param			preload		query	string	false	"Should preload torrent"
//	@Param			stat		query	string	false	"Get statistics from torrent"
//	@Param			save		query	string	false	"Should save torrent"
//	@Param			m3u			query	string	false	"Get torrent as M3U playlist"
//	@Param			fromlast	query	string	false	"Get M3U from last played file"
//	@Param			play		query	string	false	"Start stream torrent"
//	@Param			title		query	string	false	"Set title of torrent"
//	@Param			poster		query	string	false	"Set poster link of torrent"
//	@Param			category	query	string	false	"Set category of torrent, used in web: movie, tv, music, other"
//
//	@Produce		application/octet-stream
//	@Success		200	"Data returned according to query"
//	@Router			/stream [get]
func stream(c *gin.Context) {
	deps := streamDepsFromContext(c)
	f := validateStreamRequest(c)

	if handleExplicitLegacyStream(c, f) {
		return
	}

	handleLegacyStreamCompatibility(c, deps, f)
}

func handleLegacyStreamCompatibility(c *gin.Context, deps streamHandlerDeps, f streamFlags) {
	link := c.Query("link")
	notAuth := c.GetBool("auth_required") && c.GetString(gin.AuthUserKey) == ""

	if handleStreamAuth(c, link, notAuth, f.play, f.m3u) {
		return
	}

	target, ok := prepareLegacyStreamTarget(c, deps, true, f.play || f.preload)
	if !ok {
		return
	}

	// Legacy: save can be combined with play/m3u.
	if f.save {
		deps.Torrents.SaveToDB(target.tor)
	}

	if f.preload {
		target.preloadQueued = enqueueLegacyPreload(deps, target.tor, target.index)
	}

	handleLegacyStreamAction(c, deps, f, target)
}

func prepareLegacyStreamTarget(c *gin.Context, deps streamHandlerDeps, allowCreate, requireValidIndex bool) (legacyStreamTarget, bool) {
	link := c.Query("link")
	if link == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("link", "should not be empty"))

		return legacyStreamTarget{}, false
	}

	spec, meta, err := parseStreamLink(c, deps.Parser)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return legacyStreamTarget{}, false
	}

	tor, err := deps.Streams.EnsureTorrent(deps.Torrents, spec, contracts.StreamMeta{
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

	index, err := parseStreamFileIndex(c, deps.Parser, tor.FileCount())
	if err != nil && requireValidIndex {
		abortAPIError(c, http.StatusBadRequest, err)

		return legacyStreamTarget{}, false
	}

	return legacyStreamTarget{tor: tor, index: index}, true
}

func handleExplicitLegacyStream(c *gin.Context, f streamFlags) bool {
	switch {
	case f.stat && !f.play && !f.save && !f.m3u:
		streamStat(c)
	case f.m3u && !f.play && !f.save && !f.stat:
		streamM3U(c)
	case f.save && !f.play && !f.stat && !f.m3u:
		streamSave(c)
	case f.play && !f.stat && !f.m3u && !f.save:
		streamPlay(c)
	default:
		return false
	}

	return true
}

func handleLegacyStreamAction(c *gin.Context, deps streamHandlerDeps, f streamFlags, target legacyStreamTarget) {
	switch {
	case f.preload && !f.stat && !f.m3u && !f.play && !f.save:
		if !target.preloadQueued {
			abortAPIError(c, http.StatusServiceUnavailable, newInternalError("preload queue is full", nil))

			return
		}

		c.JSON(http.StatusAccepted, gin.H{"status": "preload accepted", "hash": target.tor.HashHex()})
	case f.stat:
		c.JSON(200, target.tor.Status())
	case f.m3u:
		sendLegacyStreamM3U(c, deps, target.tor, f.fromlast)
	case f.play:
		streamTorrentFile(c, target.tor, target.index)
	case f.save:
		c.Status(200)
	default:
		abortAPIError(c, http.StatusBadRequest, newValidationError("action", "no supported stream action specified"))
	}
}

func enqueueLegacyPreload(deps streamHandlerDeps, tor contracts.TorrentHandle, index int) bool {
	queued := deps.Torrents.EnqueuePreload(tor, index)
	if !queued {
		log.TLogln("preload queue is full, skipping preload")
	}

	return queued
}

func sendLegacyStreamM3U(c *gin.Context, deps streamHandlerDeps, tor contracts.TorrentHandle, fromlast bool) {
	name := deps.Helpers.NormalizePlaylistName(c.Param("fname"), tor.Name())
	host := utils2.GetScheme(c) + "://" + utils2.GetHost(c)
	m3ulist := deps.Playback.BuildM3UFromStatus(tor.Status(), host, fromlast, deps.Viewed)
	sendM3U(c, name, tor.HashHex(), m3ulist)
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

func streamNoAuth(c *gin.Context) {
	deps := streamDepsFromContext(c)
	f := validateStreamRequest(c)

	target, ok := prepareLegacyStreamTarget(c, deps, false, f.play)
	if !ok {
		return
	}

	if f.preload {
		target.preloadQueued = enqueueLegacyPreload(deps, target.tor, target.index)
	}

	if f.m3u {
		sendLegacyStreamM3U(c, deps, target.tor, f.fromlast)

		return
	}

	if f.play {
		streamTorrentFile(c, target.tor, target.index)

		return
	}

	c.Header("WWW-Authenticate", "Basic realm=Authorization Required")
	abortAPIError(c, http.StatusUnauthorized, newUnauthorizedError("authorization required"))
}
