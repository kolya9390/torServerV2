package api

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"server/internal/app/contracts"
	"time"

	"github.com/anacrolix/missinggo/v2/httptoo"

	"server/utils"

	"github.com/gin-gonic/gin"
)

// allPlayList godoc
//
//	@Summary		Get a M3U playlist with all torrents
//	@Description	Retrieve all torrents and generates a bundled M3U playlist.
//
//	@Tags			API
//
//	@Produce		audio/x-mpegurl
//	@Success		200	{file}	file
//	@Router			/playlistall/all.m3u [get]
func allPlayList(c *gin.Context) {
	deps := playlistDepsFromContext(c)
	host := utils.GetScheme(c) + "://" + utils.GetHost(c)
	res := deps.Playback.BuildAllPlaylist(host, deps.Torrents)
	sendM3U(c, res.Name, res.Hash, res.Body)
}

// playList godoc
//
//	@Summary		Get HTTP link of torrent in M3U list
//	@Description	Get HTTP link of torrent in M3U list.
//
//	@Tags			API
//
//	@Param			hash		query	string	true	"Torrent hash"
//	@Param			fromlast	query	bool	false	"From last play file"
//
//	@Produce		audio/x-mpegurl
//	@Success		200	{file}	file
//	@Router			/playlist [get]
func playList(c *gin.Context) {
	deps := playlistDepsFromContext(c)
	hash, _ := c.GetQuery("hash")
	_, fromlast := c.GetQuery("fromlast")

	if hash == "" {
		abortAPIError(c, http.StatusBadRequest, newValidationError("hash", "is required"))

		return
	}

	host := utils.GetScheme(c) + "://" + utils.GetHost(c)

	res, err := deps.Playback.BuildPlaylistByHash(hash, c.Param("fname"), fromlast, host, deps.Torrents, deps.Viewed)
	if err != nil {
		switch {
		case errors.Is(err, contracts.ErrPlaylistHashRequired):
			abortAPIError(c, http.StatusBadRequest, newValidationError("hash", "is required"))
		case errors.Is(err, contracts.ErrPlaylistTorrentNotFound):
			abortAPIError(c, http.StatusNotFound, newNotFoundError("torrent not found"))
		case errors.Is(err, contracts.ErrPlaylistLoadFailed):
			abortAPIError(c, http.StatusInternalServerError, newInternalError("failed to load torrent info", nil))
		default:
			abortAPIError(c, http.StatusInternalServerError, newInternalError("failed to build playlist", err))
		}

		return
	}

	sendM3U(c, res.Name, res.Hash, res.Body)
}

func sendM3U(c *gin.Context, name, hash string, m3u string) {
	c.Header("Content-Type", "audio/x-mpegurl")
	c.Header("Connection", "close")

	if hash != "" {
		etag := hex.EncodeToString(fmt.Appendf(nil, "%s/%s", hash, name))
		c.Header("ETag", httptoo.EncodeQuotedString(etag))
	}

	if name == "" {
		name = "playlist.m3u"
	}

	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeContent(c.Writer, c.Request, name, time.Now(), bytes.NewReader([]byte(m3u)))
}
