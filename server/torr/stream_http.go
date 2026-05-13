package torr

import (
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/anacrolix/dms/dlna"
	"github.com/anacrolix/missinggo/v2/httptoo"
	"github.com/anacrolix/torrent"

	mt "server/mimetype"
	"server/torr/storage/torrstor"
)

func streamReaderReadahead(pieceLength, cacheCap int64) int64 {
	_ = pieceLength

	ra := int64(16 << 20)
	if cacheCap > 0 && cacheCap < ra {
		return cacheCap
	}

	return ra
}

func requestedRangeStart(req *http.Request, size int64) (int64, bool) {
	if req == nil {
		return 0, false
	}

	rangeHeader := strings.TrimSpace(req.Header.Get("Range"))
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return 0, false
	}

	spec := strings.TrimSpace(strings.TrimPrefix(rangeHeader, "bytes="))
	if spec == "" || strings.Contains(spec, ",") {
		return 0, false
	}

	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return 0, false
	}

	startPart := strings.TrimSpace(parts[0])
	if startPart == "" {
		return 0, false
	}

	start, err := strconv.ParseInt(startPart, 10, 64)
	if err != nil || start < 0 {
		return 0, false
	}

	if size > 0 && start >= size {
		return 0, false
	}

	return start, true
}

func initialStreamOffset(req *http.Request, size int64) int64 {
	if start, ok := requestedRangeStart(req, size); ok {
		return start
	}

	return 0
}

// maxInt returns the maximum of two integers.
func maxInt(a, b int) int {
	if a > b {
		return a
	}

	return b
}

func findFileByID(t *Torrent, fileID int) (*torrent.File, error) {
	file := t.getFileByID(fileID)
	if file == nil {
		return nil, fmt.Errorf("file with id %v not found", fileID)
	}

	return file, nil
}

func setStreamHeaders(resp http.ResponseWriter, file *torrent.File, t *Torrent, streamTimeout int, req *http.Request) {
	if streamTimeout > 0 {
		resp.Header().Set("X-Stream-Timeout", strconv.Itoa(streamTimeout))
	}

	etagBuf := make([]byte, 0, 40+1+len(file.Path()))
	etagBuf = append(etagBuf, t.Hash().HexString()...)
	etagBuf = append(etagBuf, '/')
	etagBuf = append(etagBuf, file.Path()...)
	etag := hex.EncodeToString(etagBuf)
	resp.Header().Set("ETag", httptoo.EncodeQuotedString(etag))
	resp.Header().Set("transferMode.dlna.org", "Streaming")

	mime, err := mt.MimeTypeByPath(file.Path())
	if err == nil && mime.IsMedia() {
		resp.Header().Set("content-type", mime.String())
	}

	if req.Header.Get("getContentFeatures.dlna.org") != "" {
		resp.Header().Set("contentFeatures.dlna.org", dlna.ContentFeatures{
			SupportRange:    true,
			SupportTimeSeek: true,
		}.String())
	}

	resp.Header().Set("Accept-Ranges", "bytes")
}

func (t *Torrent) newReaderForRequest(fileID int, file *torrent.File, req *http.Request) (*torrstor.Reader, func()) {
	_ = fileID

	reader := t.NewReader(file)
	if reader == nil {
		return nil, func() {}
	}

	curSets := t.currentSettings()
	if curSets.StreamConfig().ResponsiveMode {
		reader.SetResponsive()
	}

	readahead := int64(16 << 20)
	if t.Info() != nil {
		readahead = streamReaderReadahead(t.Info().PieceLength, 0)
	}

	if t.cache != nil {
		readahead = streamReaderReadahead(0, t.cache.GetCapacity())
	}

	reader.SetReadahead(readahead)

	startOffset := initialStreamOffset(req, file.Length())
	if reader.Offset() != startOffset {
		if _, err := reader.Seek(startOffset, io.SeekStart); err != nil {
			t.CloseReader(reader)

			return nil, func() {}
		}
	}

	var closeOnce sync.Once

	closeReader := func() {
		closeOnce.Do(func() {
			t.CloseReader(reader)
		})
	}

	return reader, closeReader
}
