package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// play godoc
//
//	@Summary		Play given torrent by infohash
//	@Description	Play given torrent referenced by infohash and file id.
//
//	@Tags			API
//
//	@Param			hash		path	string	true	"Torrent infohash"
//	@Param			id			path	string	true	"File index in torrent"
//
//	@Produce		application/octet-stream
//	@Success		200	"Torrent data"
//	@Router			/play/{hash}/{id} [get]
func play(c *gin.Context) {
	svc := getServices()
	hash := c.Param("hash")
	indexStr := c.Param("id")
	notAuth := c.GetBool("auth_required") && c.GetString(gin.AuthUserKey) == ""

	target, err := svc.Playback.ResolvePlay(hash, indexStr, notAuth, svc.Torrents)
	if err != nil {
		switch {
		case errors.Is(err, ErrPlayPathRequired):
			abortAPIError(c, http.StatusNotFound, newValidationError("path", "hash and id are required"))
		case errors.Is(err, ErrPlayHashInvalid):
			abortAPIError(c, http.StatusBadRequest, newValidationError("hash", "invalid infohash"))
		case errors.Is(err, ErrPlayUnauthorized):
			c.Header("WWW-Authenticate", "Basic realm=Authorization Required")
			abortAPIError(c, http.StatusUnauthorized, newUnauthorizedError("authorization required"))
		case errors.Is(err, ErrPlayTorrentNotFound):
			abortAPIError(c, http.StatusNotFound, newNotFoundError("torrent not active"))
		case errors.Is(err, ErrPlayLoadFailed):
			abortAPIError(c, http.StatusInternalServerError, newInternalError("failed to load torrent from db", err))
		case errors.Is(err, ErrPlayTimeout):
			abortAPIError(c, http.StatusInternalServerError, newInternalError("torrent connection timeout", nil))
		case errors.Is(err, ErrPlayFileIndexInvalid):
			abortAPIError(c, http.StatusBadRequest, newValidationError("id", "invalid file index"))
		default:
			abortAPIError(c, http.StatusInternalServerError, newInternalError("failed to prepare playback", err))
		}

		return
	}

	if err := target.Torrent.Stream(target.FileIndex, c.Request, c.Writer); err != nil {
		_ = c.Error(err)
	}
}
