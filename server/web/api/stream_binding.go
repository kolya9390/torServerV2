package api

import (
	"errors"

	"server/internal/app/contracts"

	"github.com/gin-gonic/gin"
)

type streamLinkRequest struct {
	Spec contracts.TorrentSpec
	Meta streamMeta
}

type streamM3URequest struct {
	streamLinkRequest
	FromLast bool
	RawName  string
}

type streamPlayRequest struct {
	streamLinkRequest
	Preload bool
}

func bindStreamLinkRequest(c *gin.Context, deps streamParserDeps) (streamLinkRequest, error) {
	spec, meta, err := deps.Parser.ParseLink(c.Query("link"), c.Query("title"), c.Query("poster"), c.Query("category"))
	if err != nil {
		switch {
		case errors.Is(err, contracts.ErrStreamLinkEmpty):
			return streamLinkRequest{}, newValidationError("link", "should not be empty")
		case errors.Is(err, contracts.ErrStreamInvalidTorrsHash):
			return streamLinkRequest{}, newValidationError("link", "invalid torrs hash")
		default:
			return streamLinkRequest{}, newValidationError("link", "invalid magnet/hash/link")
		}
	}

	return streamLinkRequest{
		Spec: spec,
		Meta: streamMeta{
			title:    meta.Title,
			poster:   meta.Poster,
			category: meta.Category,
			data:     meta.Data,
		},
	}, nil
}

func bindStreamM3URequest(c *gin.Context, deps streamParserDeps) (streamM3URequest, error) {
	linkReq, err := bindStreamLinkRequest(c, deps)
	if err != nil {
		return streamM3URequest{}, err
	}

	_, fromLast := c.GetQuery("fromlast")

	return streamM3URequest{
		streamLinkRequest: linkReq,
		FromLast:          fromLast,
		RawName:           c.Param("fname"),
	}, nil
}

func bindStreamPlayRequest(c *gin.Context, deps streamParserDeps) (streamPlayRequest, error) {
	linkReq, err := bindStreamLinkRequest(c, deps)
	if err != nil {
		return streamPlayRequest{}, err
	}

	_, preload := c.GetQuery("preload")

	return streamPlayRequest{
		streamLinkRequest: linkReq,
		Preload:           preload,
	}, nil
}

func bindStreamFileIndex(c *gin.Context, deps streamParserDeps, fileCount int) (int, error) {
	index, err := deps.Helpers.ParseFileIndex(c.Query("index"), fileCount)
	if err != nil {
		return 0, newValidationError("index", "should be valid file index")
	}

	return index, nil
}
