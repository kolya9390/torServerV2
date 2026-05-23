package api

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"
)

type torrentUploadRequest struct {
	Form   *multipart.Form
	Fields torrentUploadFields
}

type torrentUploadFields struct {
	Save     bool
	Title    string
	Category string
	Poster   string
	Data     string
}

func bindTorrentUploadRequest(c *gin.Context) (torrentUploadRequest, error) {
	limitUploadRequestBody(c)

	form, err := c.MultipartForm()
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return torrentUploadRequest{}, newUploadTooLargeError(maxTorrentUploadBodyBytes)
		}

		return torrentUploadRequest{}, newValidationError("request", "invalid multipart form")
	}

	req := torrentUploadRequest{
		Form:   form,
		Fields: parseUploadForm(form),
	}

	if len(form.File) == 0 {
		return req, newValidationError("file", "torrent file is required")
	}

	return req, nil
}

func parseUploadForm(form *multipart.Form) torrentUploadFields {
	fields := torrentUploadFields{
		Save: len(form.Value["save"]) > 0,
	}

	if len(form.Value["title"]) > 0 {
		fields.Title = form.Value["title"][0]
	}

	if len(form.Value["category"]) > 0 {
		fields.Category = form.Value["category"][0]
	}

	if len(form.Value["poster"]) > 0 {
		fields.Poster = form.Value["poster"][0]
	}

	if len(form.Value["data"]) > 0 {
		fields.Data = form.Value["data"][0]
	}

	return fields
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
