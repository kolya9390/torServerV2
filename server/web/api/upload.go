package api

import (
	"mime/multipart"
	"net/http"
	"sort"

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
func processUploadFile(
	file *multipart.FileHeader,
	deps uploadHandlerDeps,
	save bool,
	title, category, poster, data string,
) (any, error) {
	torrFile, openErr := file.Open()
	if openErr != nil {
		return nil, openErr
	}

	defer func() {
		if closeErr := torrFile.Close(); closeErr != nil {
			log.TLogln("error close uploaded file:", closeErr)
		}
	}()

	spec, parseErr := deps.Parser.ParseTorrentFile(torrFile)
	if parseErr != nil {
		return nil, parseErr
	}

	tor, addErr := deps.Commands.Add(spec, title, poster, data, category)
	if addErr != nil {
		return nil, addErr
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

	return torStatus, nil
}

// torrentUpload godoc
//
//	@Summary		Add .torrent file
//	@Description	Supports one or more files. Multipart request body is limited to 4 MiB.
//
//	@Tags			API
//
//	@Param			file	formData	file	true	"Torrent file(s) to insert"
//	@Param			save	formData	string	false	"Save to DB"
//	@Param			title	formData	string	false	"Torrent title"
//	@Param			category	formData	string	false	"Torrent category"
//	@Param			poster	formData	string	false	"Torrent poster"
//	@Param			data	formData	string	false	"Torrent data"
//
//	@Accept			multipart/form-data
//
//	@Produce		json
//	@Success		200	{object}	object	"Torrent status for one file, array of statuses for multiple files"
//	@Router			/torrent/upload [post]
func torrentUpload(c *gin.Context) {
	deps, ok := uploadDepsFromContext(c)
	if !ok {
		return
	}

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

	files := collectUploadFiles(uploadReq.Form)
	statuses := make([]any, 0, len(files))

	for _, file := range files {
		if ctxErr := c.Request.Context().Err(); ctxErr != nil {
			log.TLogln("upload request canceled:", ctxErr)

			break
		}

		log.TLogln("add .torrent", file.Filename)

		status, procErr := processUploadFile(
			file,
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

		statuses = append(statuses, status)
	}

	if len(statuses) == 0 {
		abortAPIError(c, http.StatusBadRequest, newValidationError("file", "unable to parse/upload torrent"))

		return
	}

	c.JSON(http.StatusOK, uploadResponse(statuses, len(files)))
}

func collectUploadFiles(form *multipart.Form) []*multipart.FileHeader {
	if form == nil {
		return nil
	}

	fieldNames := make([]string, 0, len(form.File))
	for fieldName := range form.File {
		fieldNames = append(fieldNames, fieldName)
	}

	sort.Strings(fieldNames)

	totalFiles := 0
	for _, fieldName := range fieldNames {
		totalFiles += len(form.File[fieldName])
	}

	files := make([]*multipart.FileHeader, 0, totalFiles)
	for _, fieldName := range fieldNames {
		files = append(files, form.File[fieldName]...)
	}

	return files
}

func uploadResponse(statuses []any, requestedFiles int) any {
	if requestedFiles == 1 && len(statuses) == 1 {
		return statuses[0]
	}

	return statuses
}
