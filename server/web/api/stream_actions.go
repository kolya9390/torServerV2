package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"server/internal/app/contracts"
	"server/log"
	utils2 "server/utils"

	"github.com/gin-gonic/gin"
)

type streamMeta struct {
	title    string
	poster   string
	category string
	data     string
}

func (meta streamMeta) toContract() contracts.StreamMeta {
	return contracts.StreamMeta{
		Title:    meta.title,
		Poster:   meta.poster,
		Category: meta.category,
		Data:     meta.data,
	}
}

type playbackAdmissionChecker interface {
	CheckPlaybackAdmission(hash string) contracts.PlaybackAdmissionDecision
}

type playbackIntentToucher interface {
	TouchPlaybackIntent()
}

func touchPlaybackIntent(tor contracts.TorrentHandle) {
	toucher, ok := tor.(playbackIntentToucher)
	if ok {
		toucher.TouchPlaybackIntent()
	}
}

func shouldTouchStatPlaybackIntent(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}

	path := c.Request.URL.Path

	return path == "/stream" || strings.HasPrefix(path, "/stream/")
}

func ensurePlaybackAdmission(c *gin.Context, service any, hash string) bool {
	checker, ok := service.(playbackAdmissionChecker)
	if !ok || hash == "" {
		return true
	}

	decision := checker.CheckPlaybackAdmission(hash)
	if decision.Allowed {
		return true
	}

	if decision.RetryAfterSec > 0 {
		c.Header("Retry-After", strconv.Itoa(decision.RetryAfterSec))
	}

	abortAPIError(c, http.StatusServiceUnavailable, APIError{
		Type:    "stream_admission_rejected",
		Message: "too many active playback streams",
		Status:  http.StatusServiceUnavailable,
	})

	return false
}

// streamStat godoc
//
//	@Summary		Get torrent runtime status
//	@Description	Read-only status endpoint. Does not create or modify torrent state.
//	@Tags			API
//	@Param			link		query	string	true	"Magnet/hash/link to torrent"
//	@Produce		application/json
//	@Success		200	{object}	contracts.TorrentStatus
//	@Router			/streams/stat [get]
func streamStat(c *gin.Context) {
	deps, ok := streamDepsFromContext(c)
	if !ok {
		return
	}

	if isNotAuthRequest(c) {
		c.Header("WWW-Authenticate", "Basic realm=Authorization Required")
		abortAPIError(c, http.StatusUnauthorized, newUnauthorizedError("authorization required"))

		return
	}

	req, err := bindStreamLinkRequest(c, deps.Parser)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	tor := deps.Torrents.Get(req.Spec.HashHex())
	if tor == nil {
		abortAPIError(c, http.StatusNotFound, newNotFoundError("torrent not active"))

		return
	}

	if deps.Torrents.IsStored(tor) {
		abortAPIError(c, http.StatusConflict, newConflictError("torrent is stored only, activate via play"))

		return
	}

	if shouldTouchStatPlaybackIntent(c) {
		touchPlaybackIntent(tor)
	}
	writeStreamStatusResponse(c, tor.Status())
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
	deps, ok := streamDepsFromContext(c)
	if !ok {
		return
	}

	req, err := bindStreamM3URequest(c, deps.Parser)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	tor := deps.Torrents.Get(req.Spec.HashHex())
	if tor == nil {
		if isNotAuthRequest(c) {
			c.Header("WWW-Authenticate", "Basic realm=Authorization Required")
			abortAPIError(c, http.StatusUnauthorized, newUnauthorizedError("authorization required"))

			return
		}

		abortAPIError(c, http.StatusNotFound, newNotFoundError("torrent not active"))

		return
	}

	if deps.Torrents.IsStored(tor) {
		abortAPIError(c, http.StatusConflict, newConflictError("torrent is stored only, activate via play"))

		return
	}

	status := tor.Status()
	if len(status.FileStats) == 0 {
		abortAPIError(c, http.StatusConflict, newConflictError("torrent info is not ready yet"))

		return
	}

	name := deps.Helpers.NormalizePlaylistName(req.RawName, tor.Name())
	host := utils2.GetScheme(c) + "://" + utils2.GetHost(c)
	m3ulist := deps.Playback.BuildM3UFromStatus(status, host, req.FromLast, deps.Viewed)
	sendM3U(c, name, tor.HashHex(), m3ulist)
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
	deps, ok := streamDepsFromContext(c)
	if !ok {
		return
	}

	req, err := bindStreamPlayRequest(c, deps.Parser)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	if !ensurePlaybackAdmission(c, deps.Torrents, req.Spec.HashHex()) {
		return
	}

	tor, err := deps.Streams.EnsureTorrent(deps.Torrents, req.Spec, req.Meta.toContract(), !isNotAuthRequest(c))
	if err != nil {
		statusCode, apiErr := mapStreamEnsureError(err)
		if statusCode == http.StatusUnauthorized {
			c.Header("WWW-Authenticate", "Basic realm=Authorization Required")
		}

		abortAPIError(c, statusCode, apiErr)

		return
	}

	index, err := bindStreamFileIndex(c, deps.Parser, tor.FileCount())
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	touchPlaybackIntent(tor)
	if req.Preload {
		if queued := deps.Torrents.EnqueuePreload(tor, index); !queued {
			log.TLogln("preload queue is full, skipping preload")
		}
	}

	if err := c.Request.Context().Err(); err != nil {
		abortAPIError(c, http.StatusRequestTimeout, newValidationError("request", "request canceled"))

		return
	}

	if err := tor.Stream(index, c.Request, c.Writer); err != nil {
		c.Error(err) //nolint:errcheck // gin adds error to context
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
	deps, ok := streamDepsFromContext(c)
	if !ok {
		return
	}

	req, err := bindStreamLinkRequest(c, deps.Parser)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	tor := deps.Torrents.Get(req.Spec.HashHex())
	if tor == nil || deps.Torrents.IsStored(tor) {
		tor, err = deps.Torrents.Add(req.Spec, req.Meta.title, req.Meta.poster, req.Meta.data, req.Meta.category)
		if err != nil {
			abortAPIError(c, http.StatusInternalServerError, newInternalError("failed to add torrent", err))

			return
		}
	}

	deps.Torrents.SaveToDB(tor)
	writeStreamSaveResponse(c, tor.HashHex())
}

func mapStreamEnsureError(err error) (int, error) {
	switch {
	case errors.Is(err, contracts.ErrStreamUnauthorized):
		return http.StatusUnauthorized, newUnauthorizedError("authorization required")
	case errors.Is(err, contracts.ErrStreamConnectionTimeout):
		return http.StatusInternalServerError, newInternalError("torrent connection timeout", nil)
	default:
		return http.StatusInternalServerError, newInternalError("failed to add torrent", err)
	}
}

func isNotAuthRequest(c *gin.Context) bool {
	return c.GetBool("auth_required") && c.GetString(gin.AuthUserKey) == ""
}
