package api

import (
	"mime/multipart"
	"net/http"

	"server/log"
	"server/web/api/utils"

	"github.com/gin-gonic/gin"
)

// parseUploadForm extracts form fields from a multipart form.
// Returns save flag and title, category, poster, data values.
func parseUploadForm(form *multipart.Form) (save bool, title, category, poster, data string) {
	save = len(form.Value["save"]) > 0

	if len(form.Value["title"]) > 0 {
		title = form.Value["title"][0]
	}

	if len(form.Value["category"]) > 0 {
		category = form.Value["category"][0]
	}

	if len(form.Value["poster"]) > 0 {
		poster = form.Value["poster"][0]
	}

	if len(form.Value["data"]) > 0 {
		data = form.Value["data"][0]
	}

	return
}

// processUploadFile handles a single uploaded torrent file.
// Returns torSet flag, torrent status, and any error encountered.
func processUploadFile(
	file *multipart.FileHeader,
	svc *APIServices,
	save bool,
	title, category, poster, data string,
) (torSet bool, status any, err error) {
	torrFile, openErr := file.Open()
	if openErr != nil {
		return false, nil, openErr
	}

	defer func() {
		if closeErr := torrFile.Close(); closeErr != nil {
			log.TLogln("error close uploaded file:", closeErr)
		}
	}()

	spec, parseErr := utils.ParseFile(torrFile)
	if parseErr != nil {
		return false, nil, parseErr
	}

	tor, addErr := svc.Torrents.Add(spec, title, poster, data, category)
	if addErr != nil {
		return false, nil, addErr
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

	return true, tor.Status(), nil
}

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
		if rmErr := form.RemoveAll(); rmErr != nil {
			log.TLogln("error cleanup multipart form:", rmErr)
		}
	}()

	if len(form.File) == 0 {
		abortAPIError(c, http.StatusBadRequest, newValidationError("file", "torrent file is required"))

		return
	}

	save, title, category, poster, data := parseUploadForm(form)

	var (
		torSet bool
		status any
	)

	for name, file := range form.File {
		if ctxErr := c.Request.Context().Err(); ctxErr != nil {
			log.TLogln("upload request canceled:", ctxErr)

			break
		}

		log.TLogln("add .torrent", name)

		var procErr error

		torSet, status, procErr = processUploadFile(file[0], svc, save, title, category, poster, data)
		if procErr != nil {
			log.TLogln("error upload torrent:", procErr)

			continue
		}

		break
	}

	if !torSet {
		abortAPIError(c, http.StatusBadRequest, newValidationError("file", "unable to parse/upload torrent"))

		return
	}

	c.JSON(200, status)
}
