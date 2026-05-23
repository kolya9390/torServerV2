package api

import (
	"mime/multipart"
	"net/http"

	"server/log"

	"github.com/gin-gonic/gin"
)

const (
	// MaxTorrentUploadBodyBytes limits the whole multipart request body before form parsing.
	// .torrent metadata files are normally small; 4 MiB leaves room for large metadata while
	// preventing unbounded multipart memory/disk use.
	maxTorrentUploadBodyBytes int64 = 4 << 20
)

// processUploadFile handles a single uploaded torrent file.
// Returns torSet flag, torrent status, and any error encountered.
func processUploadFile(
	file *multipart.FileHeader,
	deps uploadHandlerDeps,
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

	spec, parseErr := deps.Parser.ParseTorrentFile(torrFile)
	if parseErr != nil {
		return false, nil, parseErr
	}

	tor, addErr := deps.Commands.Add(spec, title, poster, data, category)
	if addErr != nil {
		return false, nil, addErr
	}

	torStatus := deps.Queries.Status(tor)
	if torStatus != nil && torStatus.Data != "" && deps.Settings.EnableDebug() {
		log.TLogln("torrent data:", torStatus.Data)
	}

	if torStatus != nil && torStatus.Category != "" && deps.Settings.EnableDebug() {
		log.TLogln("torrent category:", torStatus.Category)
	}

	if queued := deps.Commands.EnqueueMetadataFinalize(tor, &spec, save); !queued {
		log.TLogln("metadata finalize queue is full, skipping async finalize")
	}

	return true, torStatus, nil
}

// torrentUpload godoc
//
//	@Summary		Add .torrent file
//	@Description	Only one file support. Multipart request body is limited to 4 MiB.
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
//	@Success		200	{object}	object	"Torrent status"
//	@Router			/torrent/upload [post]
func torrentUpload(c *gin.Context) {
	deps := uploadDepsFromContext(c)

	uploadReq, err := bindTorrentUploadRequest(c)

	defer func() {
		if uploadReq.Form == nil {
			return
		}

		if rmErr := uploadReq.Form.RemoveAll(); rmErr != nil {
			log.TLogln("error cleanup multipart form:", rmErr)
		}
	}()

	if err != nil {
		abortAPIError(c, http.StatusBadRequest, err)

		return
	}

	var (
		torSet bool
		status any
	)

	for name, file := range uploadReq.Form.File {
		if ctxErr := c.Request.Context().Err(); ctxErr != nil {
			log.TLogln("upload request canceled:", ctxErr)

			break
		}

		log.TLogln("add .torrent", name)

		var procErr error

		torSet, status, procErr = processUploadFile(
			file[0],
			deps,
			uploadReq.Fields.Save,
			uploadReq.Fields.Title,
			uploadReq.Fields.Category,
			uploadReq.Fields.Poster,
			uploadReq.Fields.Data,
		)
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
