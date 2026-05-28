package api

import (
	"github.com/gin-gonic/gin"
)

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
	deps, ok := streamDepsFromContext(c)
	if !ok {
		return
	}

	newLegacyStreamAdapter(deps).Handle(c)
}
