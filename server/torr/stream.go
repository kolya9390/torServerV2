package torr

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/anacrolix/dms/dlna"
	"github.com/anacrolix/missinggo/v2/httptoo"
	"github.com/anacrolix/torrent"

	"server/log"
	mt "server/mimetype"
	sets "server/settings"
	"server/torr/state"
	"server/torr/storage/torrstor"
)

// activeStreams counts currently active streaming connections.
var activeStreams int32

// streamAdmission controls concurrent stream limiting.
type streamAdmission struct {
	maxStreams   int32
	waitDuration time.Duration
}

var admission = &streamAdmission{
	maxStreams:   int32(maxInt(1, runtime.GOMAXPROCS(0))),
	waitDuration: 3 * time.Second,
}

// tryAcquireStream attempts to acquire a streaming slot.
// It returns a release function and an error if the limit is exceeded.
func tryAcquireStream(ctx context.Context) (func(), error) {
	if atomic.LoadInt32(&activeStreams) >= admission.maxStreams {
		// Try to wait for a slot with timeout
		deadline := time.After(admission.waitDuration)

		ticker := time.NewTicker(250 * time.Millisecond)

		defer ticker.Stop()

	waitLoop:
		for {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-deadline:
				return nil, errors.New("stream limit exceeded, try again later")
			case <-ticker.C:
				if atomic.LoadInt32(&activeStreams) < admission.maxStreams {
					break waitLoop
				}
			}
		}
	}

	atomic.AddInt32(&activeStreams, 1)

	release := func() {
		atomic.AddInt32(&activeStreams, -1)
	}

	return release, nil
}

// maxInt returns the maximum of two integers.
func maxInt(a, b int) int {
	if a > b {
		return a
	}

	return b
}

// findFileByID looks up a file by its numeric ID within the torrent.
// Returns an error suitable for HTTP responses when the file is not found.
func findFileByID(t *Torrent, fileID int) (*torrent.File, error) {
	file := t.getFileByID(fileID)
	if file == nil {
		return nil, fmt.Errorf("file with id %v not found", fileID)
	}

	return file, nil
}

// setStreamHeaders configures HTTP response headers for streaming.
// It sets connection, timeout, ETag, DLNA, MIME type, and range request headers.
// The req parameter is used to inspect request headers for DLNA and range support.
func setStreamHeaders(resp http.ResponseWriter, file *torrent.File, t *Torrent, streamTimeout int, req *http.Request) {
	resp.Header().Set("Connection", "close")

	if streamTimeout > 0 {
		resp.Header().Set("X-Stream-Timeout", strconv.Itoa(streamTimeout))
	}

	// Build ETag using byte slice append to avoid fmt.Sprintf alloc
	etagBuf := make([]byte, 0, 40+1+len(file.Path()))
	etagBuf = append(etagBuf, t.Hash().HexString()...)
	etagBuf = append(etagBuf, '/')
	etagBuf = append(etagBuf, file.Path()...)
	etag := hex.EncodeToString(etagBuf)
	resp.Header().Set("ETag", httptoo.EncodeQuotedString(etag))

	// DLNA headers
	resp.Header().Set("transferMode.dlna.org", "Streaming")

	// Add MIME type for media files
	mime, err := mt.MimeTypeByPath(file.Path())
	if err == nil && mime.IsMedia() {
		resp.Header().Set("content-type", mime.String())
	}

	// DLNA seek support
	if req.Header.Get("getContentFeatures.dlna.org") != "" {
		resp.Header().Set("contentFeatures.dlna.org", dlna.ContentFeatures{
			SupportRange:    true,
			SupportTimeSeek: true,
		}.String())
	}

	// Range request support
	if req.Header.Get("Range") != "" {
		resp.Header().Set("Accept-Ranges", "bytes")
	}
}

// newReaderWithTimeout creates a torrent reader and sets up disconnect monitoring.
// Returns the reader and a cancel function for the monitoring goroutine.
func (t *Torrent) newReaderWithTimeout(file *torrent.File, streamTimeout int, req *http.Request) (*torrstor.Reader, context.CancelFunc) {
	reader := t.NewReader(file)

	if reader != nil && sets.BTsets.ResponsiveMode {
		reader.SetResponsive()
	}

	ctx, cancel := context.WithCancel(req.Context())

	go func() {
		select {
		case <-ctx.Done():
			t.CloseReader(reader)
		case <-time.After(time.Duration(streamTimeout) * time.Second):
			t.CloseReader(reader)
		}
	}()

	return reader, cancel
}

// Stream serves a torrent file over HTTP with DLNA support and range requests.
// It handles concurrent streaming with admission control and proper resource cleanup.
func (t *Torrent) Stream(fileID int, req *http.Request, resp http.ResponseWriter) error {
	if t.Stat == state.TorrentClosed {
		return errors.New("torrent is closed")
	}

	release, err := tryAcquireStream(req.Context())
	if err != nil {
		retrySec := int(admission.waitDuration.Seconds())
		resp.Header().Set("Retry-After", strconv.Itoa(retrySec))
		http.Error(resp, "Too many active streams", http.StatusServiceUnavailable)

		return err
	}
	defer release()

	streamTimeout := sets.BTsets.TorrentDisconnectTimeout

	if !t.GotInfo() {
		http.NotFound(resp, req)

		return errors.New("torrent doesn't have info yet")
	}

	file, err := findFileByID(t, fileID)
	if err != nil {
		return err
	}

	if sets.MaxSize > 0 && file.Length() > sets.MaxSize {
		log.TLogln("File size exceeded:", file.DisplayPath(), file.Length(), "max:", sets.MaxSize)
		http.Error(resp, fmt.Sprintf("file size exceeded max allowed %d bytes", sets.MaxSize), http.StatusForbidden)

		return fmt.Errorf("file size exceeded max allowed %d bytes", sets.MaxSize)
	}

	reader, cancel := t.newReaderWithTimeout(file, streamTimeout, req)
	if reader == nil {
		cancel()

		return errors.New("cannot create torrent reader")
	}

	defer t.CloseReader(reader)
	defer cancel()

	streamID := atomic.LoadInt32(&activeStreams)
	host, port, clerr := net.SplitHostPort(req.RemoteAddr)

	if sets.BTsets.EnableDebug {
		if clerr != nil {
			log.TLogln("[Stream:", streamID, "] Connect client")
		} else {
			log.TLogln("[Stream:", streamID, "] Connect", host+":"+port)
		}
	}

	sets.SetViewed(&sets.Viewed{
		Hash:      t.Hash().HexString(),
		FileIndex: fileID,
	})

	setStreamHeaders(resp, file, t, streamTimeout, req)

	http.ServeContent(resp, req, file.Path(), time.Unix(t.Timestamp, 0), reader)

	if sets.BTsets.EnableDebug {
		if clerr != nil {
			log.TLogln("[Stream:", streamID, "] Disconnect client")
		} else {
			log.TLogln("[Stream:", streamID, "] Disconnect client", host+":"+port)
		}
	}

	return nil
}

// GetActiveStreams returns number of currently active streams.
func GetActiveStreams() int32 {
	return atomic.LoadInt32(&activeStreams)
}
