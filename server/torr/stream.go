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

	logStreamLifecycle := debugCfg.EnableDebug && !strings.HasPrefix(req.Header.Get("Range"), "bytes=")
	streamID := atomic.LoadInt32(&activeStreams)
	logStreamConnect(logStreamLifecycle, streamID, req.RemoteAddr)

	sets.SetViewed(&sets.Viewed{
		Hash:      t.Hash().HexString(),
		FileIndex: fileID,
	})

	setStreamHeaders(resp, file, t, streamTimeout, req)

	content := newServeContentReadSeeker(reader, file.Length())
	streamStarted := time.Now()

	fairness, releaseFairness := registerStreamFairness(streamStarted)
	defer releaseFairness()

	metricsWriter := &streamMetricsWriter{
		ResponseWriter: resp,
		trackReadWait:  debugCfg.EnableDebug,
		fairness:       fairness,
	}

	if debugCfg.EnableDebug {
		delivery, releaseDelivery := registerStreamDelivery(streamStarted)
		metricsWriter.delivery = delivery
		defer releaseDelivery()
	}

	http.ServeContent(metricsWriter, req, file.Path(), time.Unix(t.Timestamp, 0), content)
	markStreamActivity()

	if debugCfg.EnableDebug {
		recordStreamCompleted(
			streamFirstByteDuration(streamStarted, metricsWriter),
			metricsWriter.bytesWritten.Load(),
			metricsWriter.err(),
		)
	}

	logStreamDisconnect(logStreamLifecycle, streamID, req.RemoteAddr)
	logStreamMetrics(debugCfg.EnableDebug, streamID, streamStarted, metricsWriter)

	return nil
}

func logStreamConnect(enabled bool, streamID int32, remoteAddr string) {
	if !enabled {
		return
	}

	if host, port, err := net.SplitHostPort(remoteAddr); err == nil {
		log.Debug("stream connect", "stream_id", streamID, "remote_addr", host+":"+port)

		return
	}

	log.Debug("stream connect", "stream_id", streamID)
}

func logStreamDisconnect(enabled bool, streamID int32, remoteAddr string) {
	if !enabled {
		return
	}

	if host, port, err := net.SplitHostPort(remoteAddr); err == nil {
		log.Debug("stream disconnect", "stream_id", streamID, "remote_addr", host+":"+port)

		return
	}

	log.Debug("stream disconnect", "stream_id", streamID)
}

func logStreamMetrics(enabled bool, streamID int32, streamStarted time.Time, metricsWriter *streamMetricsWriter) {
	if !enabled {
		return
	}

	firstByteMS := int64(-1)
	firstByteDuration := streamFirstByteDuration(streamStarted, metricsWriter)

	if firstByteDuration > 0 {
		firstByteMS = firstByteDuration.Milliseconds()
	}

	log.DebugSampled(
		"stream.metrics",
		50,
		"stream metrics",
		"stream_id", streamID,
		"first_byte_ms", firstByteMS,
		"bytes_written", metricsWriter.bytesWritten.Load(),
	)
}

func streamFirstByteDuration(streamStarted time.Time, metricsWriter *streamMetricsWriter) time.Duration {
	firstByteNS := metricsWriter.firstWriteUnixNano.Load()
	if firstByteNS == 0 {
		return 0
	}

	return time.Unix(0, firstByteNS).Sub(streamStarted)
}
