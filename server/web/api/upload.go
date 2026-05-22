package api

import (
	"errors"
	"fmt"
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
	limitUploadRequestBody(c)

	form, err := c.MultipartForm()
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			abortAPIError(c, http.StatusRequestEntityTooLarge, newUploadTooLargeError(maxTorrentUploadBodyBytes))

			return
		}

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

		torSet, status, procErr = processUploadFile(file[0], deps, save, title, category, poster, data)
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

func limitUploadRequestBody(c *gin.Context) {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxTorrentUploadBodyBytes)
}

func newUploadTooLargeError(limit int64) error {
	return APIError{
		Type:    "validation_error",
		Message: "multipart upload exceeds maximum allowed size",
		Status:  http.StatusRequestEntityTooLarge,
		Field:   "request",
		Cause:   errors.New(formatByteLimit(limit)),
	}
}

func formatByteLimit(limit int64) string {
	const bytesInMiB = 1024 * 1024

	if limit%bytesInMiB == 0 {
		return fmt.Sprintf("max upload size is %d MiB", limit/bytesInMiB)
	}

	return fmt.Sprintf("max upload size is %d bytes", limit)
}
