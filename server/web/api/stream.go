package api

import (
	"net/http"

	"server/log"
	utils2 "server/utils"
	"server/web/api/utils"

	"github.com/gin-gonic/gin"
)

// get stat
// http://127.0.0.1:8090/stream/fname?link=...&stat
// get m3u
// http://127.0.0.1:8090/stream/fname?link=...&index=1&m3u
// http://127.0.0.1:8090/stream/fname?link=...&index=1&m3u&fromlast
// stream torrent
// http://127.0.0.1:8090/stream/fname?link=...&index=1&play
// http://127.0.0.1:8090/stream/fname?link=...&index=1&play&preload
// http://127.0.0.1:8090/stream/fname?link=...&index=1&play&save
// http://127.0.0.1:8090/stream/fname?link=...&index=1&play&save&title=...&poster=...
// only save
// http://127.0.0.1:8090/stream/fname?link=...&save&title=...&poster=...

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
	svc := getServices()
	link := c.Query("link")
	_, preload := c.GetQuery("preload")
	_, stat := c.GetQuery("stat")
	_, save := c.GetQuery("save")
	_, m3u := c.GetQuery("m3u")
	_, fromlast := c.GetQuery("fromlast")
	_, play := c.GetQuery("play")

	// Backward-compatibility layer:
	// route simple/explicit legacy intents to new separated endpoints.
	if stat && !play && !save && !m3u {
		streamStat(c)

		return
	}

	if m3u && !play && !save && !stat {
		streamM3U(c)

		return
	}

	if save && !play && !stat && !m3u {
		streamSave(c)

		return
	}

	if play && !stat && !m3u && !save {
		streamPlay(c)

		return
	}

	// Legacy compat: if preload is present without explicit play,
	// treat it as play+preload (original TorrServer behavior).
	if preload && !stat && !m3u && !save {
		streamPlay(c)

		return
	}

	notAuth := c.GetBool("auth_required") && c.GetString(gin.AuthUserKey) == ""

	if notAuth {
		err := utils.TestLink(link, !notAuth)
		if err != nil {
			abortAPIError(c, http.StatusBadRequest, newValidationError("link", "wrong link"))

			return
		}
	}

	if notAuth && (play || m3u) {
		streamNoAuth(c)

		return
	}

	if notAuth {
		c.Header("WWW-Authenticate", "Basic realm=Authorization Required")
		abortAPIError(c, http.StatusUnauthorized, newUnauthorizedError("authorization required"))

		return
	}

	if link == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("link", "should not be empty"))

		return
	}

	spec, meta, err := parseStreamLink(c)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	tor, err := svc.Streams.EnsureTorrent(svc.Torrents, spec, StreamMeta{
		Title:    meta.title,
		Poster:   meta.poster,
		Category: meta.category,
		Data:     meta.data,
	}, true)
	if err != nil {
		statusCode, apiErr := mapStreamEnsureError(err)
		abortAPIError(c, statusCode, apiErr)

		return
	}

	// legacy behavior: save can be combined with play/m3u.
	if save {
		svc.Torrents.SaveToDB(tor)
	}

	index, err := parseStreamFileIndex(c, len(tor.Files()))
	if err != nil && play {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	if preload {
		if queued := svc.Torrents.EnqueuePreload(tor, index); !queued {
			log.TLogln("preload queue is full, skipping preload")
		}
	}

	if stat {
		c.JSON(200, tor.Status())

		return
	}

	if m3u {
		name := svc.Streams.NormalizePlaylistName(c.Param("fname"), tor.Name())
		host := utils2.GetScheme(c) + "://" + utils2.GetHost(c)
		m3ulist := svc.Playback.BuildM3UFromStatus(tor.Status(), host, fromlast, svc.Viewed)
		sendM3U(c, name, tor.Hash().HexString(), m3ulist)

		return
	}

	if play {
		if err := c.Request.Context().Err(); err != nil {
			abortAPIError(c, http.StatusRequestTimeout, newValidationError("request", "request canceled"))

			return
		}

		if err := tor.Stream(index, c.Request, c.Writer); err != nil {
			_ = c.Error(err)
		}

		return
	}

	if save {
		c.Status(200)

		return
	}

	abortAPIError(c, http.StatusBadRequest, newValidationError("action", "no supported stream action specified"))
}

func streamNoAuth(c *gin.Context) {
	svc := getServices()
	link := c.Query("link")
	_, preload := c.GetQuery("preload")
	_, m3u := c.GetQuery("m3u")
	_, fromlast := c.GetQuery("fromlast")
	_, play := c.GetQuery("play")

	if link == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("link", "should not be empty"))

		return
	}

	spec, meta, err := parseStreamLink(c)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	tor, err := svc.Streams.EnsureTorrent(svc.Torrents, spec, StreamMeta{
		Title:    meta.title,
		Poster:   meta.poster,
		Category: meta.category,
		Data:     meta.data,
	}, false)
	if err != nil {
		statusCode, apiErr := mapStreamEnsureError(err)
		if statusCode == http.StatusUnauthorized {
			c.Header("WWW-Authenticate", "Basic realm=Authorization Required")
		}

		abortAPIError(c, statusCode, apiErr)

		return
	}

	index, err := parseStreamFileIndex(c, len(tor.Files()))
	if err != nil && play {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	if preload {
		if queued := svc.Torrents.EnqueuePreload(tor, index); !queued {
			log.TLogln("preload queue is full, skipping preload")
		}
	}

	if m3u {
		name := svc.Streams.NormalizePlaylistName(c.Param("fname"), tor.Name())
		host := utils2.GetScheme(c) + "://" + utils2.GetHost(c)
		m3ulist := svc.Playback.BuildM3UFromStatus(tor.Status(), host, fromlast, svc.Viewed)
		sendM3U(c, name, tor.Hash().HexString(), m3ulist)

		return
	}

	if play {
		if err := c.Request.Context().Err(); err != nil {
			abortAPIError(c, http.StatusRequestTimeout, newValidationError("request", "request canceled"))

			return
		}

		if err := tor.Stream(index, c.Request, c.Writer); err != nil {
			_ = c.Error(err)
		}

		return
	}

	c.Header("WWW-Authenticate", "Basic realm=Authorization Required")
	abortAPIError(c, http.StatusUnauthorized, newUnauthorizedError("authorization required"))
}
