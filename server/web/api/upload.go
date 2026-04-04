package api

import (
	"net/http"

	"server/log"
	"server/web/api/utils"

	"github.com/gin-gonic/gin"
)

// torrentUpload godoc
//
//	@Summary		Add .torrent file
//	@Description	Only one file support.
//
//	@Tags			API
//
//	@Param			file	formData	file	true	"Torrent file to insert"
//	@Param			save	formData	string	false	"Save to DB"
//	@Param			title	formData	string	false	"Torrent title"
//	@Param			category	formData	string	false	"Torrent category"
//	@Param			poster	formData	string	false	"Torrent poster"
//	@Param			data	formData	string	false	"Torrent data"
//
//	@Accept			multipart/form-data
//
//	@Produce		json
//	@Success		200	{object}	state.TorrentStatus	"Torrent status"
//	@Router			/torrent/upload [post]
func torrentUpload(c *gin.Context) {
	svc := getServices()
	form, err := c.MultipartForm()
	if err != nil {
		abortAPIError(c, http.StatusBadRequest, newValidationError("request", "invalid multipart form"))
		return
	}
	defer func() {
		if err := form.RemoveAll(); err != nil {
			log.TLogln("error cleanup multipart form:", err)
		}
	}()

	if len(form.File) == 0 {
		abortAPIError(c, http.StatusBadRequest, newValidationError("file", "torrent file is required"))
		return
	}

	save := len(form.Value["save"]) > 0
	title := ""
	if len(form.Value["title"]) > 0 {
		title = form.Value["title"][0]
	}
	category := ""
	if len(form.Value["category"]) > 0 {
		category = form.Value["category"][0]
	}
	poster := ""
	if len(form.Value["poster"]) > 0 {
		poster = form.Value["poster"][0]
	}
	data := ""
	if len(form.Value["data"]) > 0 {
		data = form.Value["data"][0]
	}
	var (
		torSet bool
		status interface{}
	)
	for name, file := range form.File {
		if err := c.Request.Context().Err(); err != nil {
			log.TLogln("upload request canceled:", err)
			break
		}
		log.TLogln("add .torrent", name)

		torrFile, err := file[0].Open()
		if err != nil {
			log.TLogln("error upload torrent:", err)
			continue
		}
		defer func() {
			if err := torrFile.Close(); err != nil {
				log.TLogln("error close uploaded file:", err)
			}
		}()

		spec, err := utils.ParseFile(torrFile)
		if err != nil {
			log.TLogln("error upload torrent:", err)
			continue
		}

		tor, err := svc.Torrents.Add(spec, title, poster, data, category)
		if err != nil {
			log.TLogln("error upload torrent:", err)
			continue
		}

		if tor.Data != "" && svc.Settings.EnableDebug() {
			log.TLogln("torrent data:", tor.Data)
		}
		if tor.Category != "" && svc.Settings.EnableDebug() {
			log.TLogln("torrent category:", tor.Category)
		}

		if queued := svc.Torrents.EnqueueMetadataFinalize(tor, spec, save); !queued {
			log.TLogln("metadata finalize queue is full, skipping async finalize")
		}
		status = tor.Status()
		torSet = true

		break
	}
	if !torSet {
		abortAPIError(c, http.StatusBadRequest, newValidationError("file", "unable to parse/upload torrent"))
		return
	}
	c.JSON(200, status)
}
