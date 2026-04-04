package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ffp godoc
//
//	@Summary		Gather informations using ffprobe
//	@Description	Gather informations using ffprobe.
//
//	@Tags			API
//
//	@Param			hash	path	string	true	"Torrent hash"
//	@Param			id		path	string	true	"File index in torrent"
//
//	@Produce		json
//	@Success		200	"Data returned from ffprobe"
//	@Router			/ffp/{hash}/{id} [get]
func ffp(c *gin.Context) {
	hash := c.Param("hash")
	indexStr := c.Param("id")

	if hash == "" || indexStr == "" {
		abortAPIError(c, http.StatusNotFound, newValidationError("path", "hash and id are required"))
		return
	}

	data, err := getServices().Media.ProbePlayURL(hash, indexStr)
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, newInternalError("error getting data from ffprobe", err))
		return
	}

	c.JSON(200, data)
}
