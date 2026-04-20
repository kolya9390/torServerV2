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
	"sync"
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

func currentAdmission() streamAdmission {
	maxStreams := sets.GetSettings().MaxConcurrentStreams
	if maxStreams <= 0 {
		maxStreams = maxInt(1, runtime.GOMAXPROCS(0)*2)
	}

	waitSec := sets.GetSettings().StreamQueueWaitSec
	if waitSec <= 0 {
		waitSec = 3
	}

	return streamAdmission{
		maxStreams:   int32(maxStreams),
		waitDuration: time.Duration(waitSec) * time.Second,
	}
}

func acquireStreamSlot(maxStreams int32) bool {
	for {
		current := atomic.LoadInt32(&activeStreams)
		if current >= maxStreams {
			return false
		}

		if atomic.CompareAndSwapInt32(&activeStreams, current, current+1) {
			return true
		}
	}
}

// tryAcquireStream attempts to acquire a streaming slot.
// It returns a release function and an error if the limit is exceeded.
func tryAcquireStream(ctx context.Context) (func(), error) {
	admission := currentAdmission()

	if !acquireStreamSlot(admission.maxStreams) {
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
				if acquireStreamSlot(admission.maxStreams) {
					break waitLoop
				}
			}
		}
	}

	release := func() {
		if atomic.AddInt32(&activeStreams, -1) < 0 {
			atomic.StoreInt32(&activeStreams, 0)
		}
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

// newReaderForRequest creates a torrent reader and closes it when the HTTP client disconnects.
// The configured torrent disconnect timeout is an idle/lifecycle timeout, not a hard stream duration limit.
func (t *Torrent) newReaderForRequest(file *torrent.File, req *http.Request) (*torrstor.Reader, func(), context.CancelFunc) {
	reader := t.NewReader(file)
	if reader == nil {
		return nil, func() {}, func() {}
	}

	if sets.BTsets.ResponsiveMode {
		reader.SetResponsive()
	}

	ctx, cancel := context.WithCancel(req.Context())
	var closeOnce sync.Once
	closeReader := func() {
		closeOnce.Do(func() {
			t.CloseReader(reader)
		})
	}

	go func() {
		<-ctx.Done()
		closeReader()
	}()

	return reader, closeReader, cancel
}

// Stream serves a torrent file over HTTP with DLNA support and range requests.
// It handles concurrent streaming with admission control and proper resource cleanup.
func (t *Torrent) Stream(fileID int, req *http.Request, resp http.ResponseWriter) error {
	if t.Stat == state.TorrentClosed {
		return errors.New("torrent is closed")
	}

	release, err := tryAcquireStream(req.Context())
	if err != nil {
		retrySec := int(currentAdmission().waitDuration.Seconds())
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

	reader, closeReader, cancel := t.newReaderForRequest(file, req)
	if reader == nil {
		cancel()

		return errors.New("cannot create torrent reader")
	}

	defer closeReader()
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
