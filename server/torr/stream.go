package torr

import (
	"errors"
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

	t.TouchPlaybackIntent()

	curSets := t.currentSettings()
	admission := currentAdmission(curSets)
	debugCfg := curSets.DebugConfig()
	debugEnabled := debugCfg.EnableDebug
	torrentID := streamTorrentRuntimeID(t)

	release, err := tryAcquireStream(
		req.Context(),
		curSets,
		t.Hash().HexString(),
		torrentID,
		debugEnabled,
	)
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

	file, err := t.streamFileForRequest(fileID, serverCfg.MaxSize, req, resp)
	if err != nil {
		return err
	}

	reader, closeReader := t.newReaderForRequest(fileID, file, req)
	if reader == nil {
		return errors.New("cannot create torrent reader")
	}

	defer closeReader()

	sets.SetViewed(&sets.Viewed{
		Hash:      t.Hash().HexString(),
		FileIndex: fileID,
	})

	setStreamHeaders(resp, file, t, streamTimeout, req)

	deliveryMetadata := streamDeliveryMetadata{
		initialOffset:  reader.Offset(),
		fileSize:       file.Length(),
		requestedRange: req.Header.Get("Range") != "",
	}
	if debugEnabled {
		deliveryMetadata.startupState = streamStartupStateProviderFor(t.cache, reader)
	}

	instrumentation := startStreamInstrumentation(
		req,
		resp,
		debugEnabled,
		torrentID,
		deliveryMetadata,
	)
	defer instrumentation.release()

	if err := t.warmupPlaybackStartup(
		req,
		reader,
		file.Length(),
		debugEnabled,
		instrumentation.writer.delivery,
	); err != nil {
		return err
	}

	content := newServeContentReadSeeker(reader, file.Length(), instrumentation.writer.delivery)
	http.ServeContent(instrumentation.writer, req, file.Path(), time.Unix(t.Timestamp, 0), content)
	markStreamActivity()
	finishStreamInstrumentation(instrumentation, req.RemoteAddr)

	return nil
}

type streamInstrumentation struct {
	debugEnabled bool
	logLifecycle bool
	streamID     int32
	started      time.Time
	writer       *streamMetricsWriter
	release      func()
}

func startStreamInstrumentation(
	req *http.Request,
	resp http.ResponseWriter,
	debugEnabled bool,
	torrentID uint64,
	deliveryMetadata streamDeliveryMetadata,
) streamInstrumentation {
	started := time.Now()
	streamID := atomic.LoadInt32(&activeStreams)
	logLifecycle := debugEnabled && !strings.HasPrefix(req.Header.Get("Range"), "bytes=")
	fairness, releaseFairness := registerStreamFairness(started, torrentID)
	writer := &streamMetricsWriter{
		ResponseWriter: resp,
		trackReadWait:  debugEnabled,
		fairness:       fairness,
	}
	release := releaseFairness

	if debugEnabled {
		delivery, releaseDelivery := registerStreamDeliveryWithMetadata(started, torrentID, deliveryMetadata)
		writer.delivery = delivery
		fairness.setDelivery(delivery)

		release = func() {
			releaseDelivery()
			releaseFairness()
		}
	}

	logStreamConnect(logLifecycle, streamID, req.RemoteAddr)

	return streamInstrumentation{
		debugEnabled: debugEnabled,
		logLifecycle: logLifecycle,
		streamID:     streamID,
		started:      started,
		writer:       writer,
		release:      release,
	}
}

// streamTorrentRuntimeID is privacy-safe and process-local. It is used by runtime
// policies even when debug metrics are disabled; debug mode only controls exposure.
func streamTorrentRuntimeID(t *Torrent) uint64 {
	return t.RuntimeDiagnosticID()
}

func finishStreamInstrumentation(instrumentation streamInstrumentation, remoteAddr string) {
	if instrumentation.debugEnabled {
		recordStreamCompleted(
			streamFirstByteDuration(instrumentation.started, instrumentation.writer),
			instrumentation.writer.bytesWritten.Load(),
			instrumentation.writer.err(),
		)
	}

	logStreamDisconnect(instrumentation.logLifecycle, instrumentation.streamID, remoteAddr)
	logStreamMetrics(
		instrumentation.debugEnabled,
		instrumentation.streamID,
		instrumentation.started,
		instrumentation.writer,
	)
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
