package torr

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"server/log"
	sets "server/settings"
	"server/torr/state"
)

// Stream serves a torrent file over HTTP with DLNA support and range requests.
// It handles concurrent streaming with admission control and proper resource cleanup.
func (t *Torrent) Stream(fileID int, req *http.Request, resp http.ResponseWriter) error {
	if t.Stat == state.TorrentClosed {
		return errors.New("torrent is closed")
	}

	curSets := t.currentSettings()
	admission := currentAdmission(curSets)

	release, err := tryAcquireStream(req.Context(), curSets)
	if err != nil {
		retrySec := int(admission.waitDuration.Seconds())
		resp.Header().Set("Retry-After", strconv.Itoa(retrySec))
		http.Error(resp, "Too many active streams", http.StatusServiceUnavailable)

		return err
	}

	defer release()
	markStreamActivity()

	serverCfg := t.currentRuntimeState().ServerConfig()
	streamTimeout := curSets.TorrentDisconnectTimeout
	debugCfg := curSets.DebugConfig()

	if !t.GotInfo() {
		http.NotFound(resp, req)

		return errors.New("torrent doesn't have info yet")
	}

	file, err := findFileByID(t, fileID)
	if err != nil {
		return err
	}

	if serverCfg.MaxSize > 0 && file.Length() > serverCfg.MaxSize {
		log.TLogln("File size exceeded:", file.DisplayPath(), file.Length(), "max:", serverCfg.MaxSize)
		http.Error(resp, fmt.Sprintf("file size exceeded max allowed %d bytes", serverCfg.MaxSize), http.StatusForbidden)

		return fmt.Errorf("file size exceeded max allowed %d bytes", serverCfg.MaxSize)
	}

	reader, closeReader := t.newReaderForRequest(fileID, file, req)
	if reader == nil {
		return errors.New("cannot create torrent reader")
	}

	defer closeReader()

	streamID := atomic.LoadInt32(&activeStreams)
	host, port, clerr := net.SplitHostPort(req.RemoteAddr)

	logStreamLifecycle := debugCfg.EnableDebug && !strings.HasPrefix(req.Header.Get("Range"), "bytes=")
	if logStreamLifecycle {
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

	content := newServeContentReadSeeker(reader, file.Length())
	metricsWriter := &streamMetricsWriter{ResponseWriter: resp}
	streamStarted := time.Now()
	http.ServeContent(metricsWriter, req, file.Path(), time.Unix(t.Timestamp, 0), content)
	markStreamActivity()

	if logStreamLifecycle {
		if clerr != nil {
			log.TLogln("[Stream:", streamID, "] Disconnect client")
		} else {
			log.TLogln("[Stream:", streamID, "] Disconnect client", host+":"+port)
		}
	}

	if debugCfg.EnableDebug {
		firstByteNS := metricsWriter.firstWriteUnixNano.Load()
		firstByteMS := int64(-1)

		if firstByteNS != 0 {
			firstByteMS = time.Unix(0, firstByteNS).Sub(streamStarted).Milliseconds()
		}

		log.TLogln(
			"[Stream:", streamID, "] Metrics",
			" first_byte_ms=", firstByteMS,
			" bytes_written=", metricsWriter.bytesWritten.Load(),
		)
	}

	return nil
}
