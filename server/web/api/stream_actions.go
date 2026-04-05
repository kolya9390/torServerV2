package api

import (
	"errors"
	"net/http"

	"server/log"
	"server/torr/state"
	utils2 "server/utils"

	"github.com/anacrolix/torrent"
	"github.com/gin-gonic/gin"
)

type streamMeta struct {
	title    string
	poster   string
	category string
	data     string
}

// streamStat godoc
//
//	@Summary		Get torrent runtime status
//	@Description	Read-only status endpoint. Does not create or modify torrent state.
//	@Tags			API
//	@Param			link		query	string	true	"Magnet/hash/link to torrent"
//	@Produce		application/json
//	@Success		200	{object}	state.TorrentStatus
//	@Router			/streams/stat [get]
func streamStat(c *gin.Context) {
	svc := getServices()

	if isNotAuthRequest(c) {
		c.Header("WWW-Authenticate", "Basic realm=Authorization Required")
		abortAPIError(c, http.StatusUnauthorized, newUnauthorizedError("authorization required"))

		return
	}

	spec, _, err := parseStreamLink(c)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	tor := svc.Torrents.Get(spec.InfoHash.HexString())
	if tor == nil {
		abortAPIError(c, http.StatusNotFound, newNotFoundError("torrent not active"))

		return
	}

	if tor.Stat == state.TorrentInDB {
		abortAPIError(c, http.StatusConflict, newConflictError("torrent is stored only, activate via play"))

		return
	}

	c.JSON(http.StatusOK, tor.Status())
}

// streamM3U godoc
//
//	@Summary		Get torrent playlist
//	@Description	Read-only M3U endpoint. Does not create or modify torrent state.
//	@Tags			API
//	@Param			link		query	string	true	"Magnet/hash/link to torrent"
//	@Param			fromlast	query	string	false	"Start playlist from last viewed file"
//	@Produce		audio/x-mpegurl
//	@Success		200	{file}	file
//	@Router			/streams/m3u [get]
func streamM3U(c *gin.Context) {
	svc := getServices()

	spec, _, err := parseStreamLink(c)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	tor := svc.Torrents.Get(spec.InfoHash.HexString())
	if tor == nil {
		if isNotAuthRequest(c) {
			c.Header("WWW-Authenticate", "Basic realm=Authorization Required")
			abortAPIError(c, http.StatusUnauthorized, newUnauthorizedError("authorization required"))

			return
		}

		abortAPIError(c, http.StatusNotFound, newNotFoundError("torrent not active"))

		return
	}

	if tor.Stat == state.TorrentInDB {
		abortAPIError(c, http.StatusConflict, newConflictError("torrent is stored only, activate via play"))

		return
	}

	status := tor.Status()
	if len(status.FileStats) == 0 {
		abortAPIError(c, http.StatusConflict, newConflictError("torrent info is not ready yet"))

		return
	}

	_, fromlast := c.GetQuery("fromlast")
	name := svc.Streams.NormalizePlaylistName(c.Param("fname"), tor.Name())
	host := utils2.GetScheme(c) + "://" + utils2.GetHost(c)
	m3ulist := svc.Playback.BuildM3UFromStatus(status, host, fromlast, svc.Viewed)
	sendM3U(c, name, tor.Hash().HexString(), m3ulist)
}

// streamPlay godoc
//
//	@Summary		Play torrent file
//	@Description	Command endpoint for streaming torrent file by index.
//	@Tags			API
//	@Param			link		query	string	true	"Magnet/hash/link to torrent"
//	@Param			index		query	string	true	"File index in torrent"
//	@Param			preload		query	string	false	"Should preload torrent before stream"
//	@Param			title		query	string	false	"Torrent title"
//	@Param			poster		query	string	false	"Poster URL"
//	@Param			category	query	string	false	"Torrent category"
//	@Produce		application/octet-stream
//	@Success		200	"Torrent data"
//	@Router			/streams/play [get]
func streamPlay(c *gin.Context) {
	svc := getServices()

	log.TLogln("[DEBUG] streamPlay: starting")

	spec, meta, err := parseStreamLink(c)
	if err != nil {
		log.TLogln("[DEBUG] streamPlay: parseStreamLink error:", err)
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	log.TLogln("[DEBUG] streamPlay: spec parsed, calling EnsureTorrent")

	tor, err := svc.Streams.EnsureTorrent(svc.Torrents, spec, StreamMeta{
		Title:    meta.title,
		Poster:   meta.poster,
		Category: meta.category,
		Data:     meta.data,
	}, !isNotAuthRequest(c))
	if err != nil {
		statusCode, apiErr := mapStreamEnsureError(err)
		if statusCode == http.StatusUnauthorized {
			c.Header("WWW-Authenticate", "Basic realm=Authorization Required")
		}

		abortAPIError(c, statusCode, apiErr)

		return
	}

	index, err := parseStreamFileIndex(c, len(tor.Files()))
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	_, preload := c.GetQuery("preload")
	if preload {
		if queued := svc.Torrents.EnqueuePreload(tor, index); !queued {
			log.TLogln("preload queue is full, skipping preload")
		}
	}

	if err := c.Request.Context().Err(); err != nil {
		abortAPIError(c, http.StatusRequestTimeout, newValidationError("request", "request canceled"))

		return
	}

	if err := tor.Stream(index, c.Request, c.Writer); err != nil {
		_ = c.Error(err)
	}
}

// streamSave godoc
//
//	@Summary		Save torrent metadata to DB
//	@Description	Command endpoint that saves torrent metadata without streaming.
//	@Tags			API
//	@Param			link		query	string	true	"Magnet/hash/link to torrent"
//	@Param			title		query	string	false	"Torrent title"
//	@Param			poster		query	string	false	"Poster URL"
//	@Param			category	query	string	false	"Torrent category"
//	@Produce		application/json
//	@Success		200	{object}	map[string]interface{}
//	@Router			/streams/save [post]
func streamSave(c *gin.Context) {
	svc := getServices()

	spec, meta, err := parseStreamLink(c)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	tor := svc.Torrents.Get(spec.InfoHash.HexString())
	if tor == nil || tor.Stat == state.TorrentInDB {
		tor, err = svc.Torrents.Add(spec, meta.title, meta.poster, meta.data, meta.category)
		if err != nil {
			abortAPIError(c, http.StatusInternalServerError, newInternalError("failed to add torrent", err))

			return
		}
	}

	if tor.Title == "" && tor.Name() != "" {
		tor.Title = tor.Name()
	}

	svc.Torrents.SaveToDB(tor)
	c.JSON(http.StatusOK, gin.H{"status": "saved", "hash": tor.Hash().HexString()})
}

func parseStreamLink(c *gin.Context) (*torrent.TorrentSpec, streamMeta, error) {
	svc := getServices()

	spec, meta, err := svc.Streams.ParseLink(c.Query("link"), c.Query("title"), c.Query("poster"), c.Query("category"))
	if err != nil {
		switch {
		case errors.Is(err, ErrStreamLinkEmpty):
			return nil, streamMeta{}, newValidationError("link", "should not be empty")
		case errors.Is(err, ErrStreamInvalidTorrsHash):
			return nil, streamMeta{}, newValidationError("link", "invalid torrs hash")
		default:
			return nil, streamMeta{}, newValidationError("link", "invalid magnet/hash/link")
		}
	}

	return spec, streamMeta{title: meta.Title, poster: meta.Poster, category: meta.Category, data: meta.Data}, nil
}

func parseStreamFileIndex(c *gin.Context, fileCount int) (int, error) {
	svc := getServices()

	index, err := svc.Streams.ParseFileIndex(c.Query("index"), fileCount)
	if err != nil {
		return 0, newValidationError("index", "should be valid file index")
	}

	return index, nil
}

func mapStreamEnsureError(err error) (int, error) {
	switch {
	case errors.Is(err, ErrStreamUnauthorized):
		return http.StatusUnauthorized, newUnauthorizedError("authorization required")
	case errors.Is(err, ErrStreamConnectionTimeout):
		return http.StatusInternalServerError, newInternalError("torrent connection timeout", nil)
	default:
		return http.StatusInternalServerError, newInternalError("failed to add torrent", err)
	}
}

func isNotAuthRequest(c *gin.Context) bool {
	return c.GetBool("auth_required") && c.GetString(gin.AuthUserKey) == ""
}
