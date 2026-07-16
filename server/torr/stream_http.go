package torr

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/dms/dlna"
	"github.com/anacrolix/missinggo/v2/httptoo"
	"github.com/anacrolix/torrent"

	"server/log"
	mt "server/mimetype"
	"server/settings"
	"server/torr/storage/torrstor"
)

const (
	defaultStreamReaderReadahead        = int64(16 << 20)
	minConcurrentStreamReaderReadahead  = int64(4 << 20)
	concurrentStreamReadaheadShareRatio = int64(4)
	startupWarmupDefaultMaxBytes        = int64(8 << 20)
	startupWarmupHeavyFileThreshold     = int64(80 << 30)
	startupWarmupHeavyFileMaxBytes      = int64(32 << 20)
	startupWarmupCacheShareRatio        = int64(8)
	startupWarmupHeavyCacheShareRatio   = int64(2)
	startupWarmupMaxInitialOffset       = int64(16 << 20)
	startupWarmupTimeout                = 3 * time.Second
)

type streamReadAheadPolicyInput struct {
	pieceLength   int64
	cacheCapacity int64
	activeReaders int
}

type streamReadAheadPolicy struct {
	readerReadahead int64
}

type streamReaderContextSetter interface {
	SetContext(context.Context)
}

type playbackStartupWarmupState struct {
	preloadTargetBytes int64
	preloadedBytes     int64
	cacheCapacityBytes int64
}

type playbackStartupWarmupDecision struct {
	eligible    bool
	targetBytes int64
	skipReason  string
}

type playbackStartupWarmupResult struct {
	readBytes      int64
	elapsed        time.Duration
	outcome        string
	offsetRestored bool
}

type playbackStartupWarmupReader interface {
	io.ReadSeeker
	Offset() int64
	SetContext(context.Context)
}

func streamReaderReadahead(pieceLength, cacheCap int64, activeReaders int) int64 {
	return streamReadAheadPolicyFor(streamReadAheadPolicyInput{
		pieceLength:   pieceLength,
		cacheCapacity: cacheCap,
		activeReaders: activeReaders,
	}).readerReadahead
}

func initialPlaybackReaderReadahead(
	pieceLength int64,
	cacheCap int64,
	activeReaders int,
	playbackTorrents int,
	cfg settings.StreamConfig,
) int64 {
	if cacheCap <= 0 {
		return streamReaderReadahead(pieceLength, cacheCap, activeReaders)
	}

	readahead := adaptiveReadahead(cacheCap, playbackTorrents, cfg)
	if readahead <= 0 {
		return streamReaderReadahead(pieceLength, cacheCap, activeReaders)
	}

	if activeReaders < 1 {
		activeReaders = 1
	}

	perReaderCap := cacheCap / int64(activeReaders)
	if perReaderCap <= 0 {
		return 0
	}

	if readahead > perReaderCap {
		return perReaderCap
	}

	return readahead
}

func streamReadAheadPolicyFor(input streamReadAheadPolicyInput) streamReadAheadPolicy {
	return streamReadAheadPolicy{
		readerReadahead: boundedStreamReaderReadahead(input),
	}
}

func boundedStreamReaderReadahead(input streamReadAheadPolicyInput) int64 {
	pieceLength := input.pieceLength
	cacheCap := input.cacheCapacity
	activeReaders := input.activeReaders

	if activeReaders < 1 {
		activeReaders = 1
	}

	ra := defaultStreamReaderReadahead
	minRA := minConcurrentStreamReaderReadahead

	if pieceLength > 0 && pieceLength*2 > minRA {
		minRA = pieceLength * 2
	}

	if minRA > defaultStreamReaderReadahead {
		minRA = defaultStreamReaderReadahead
	}

	if activeReaders > 1 && cacheCap > 0 {
		perReaderCap := cacheCap / int64(activeReaders)
		if perReaderCap <= 0 {
			return 0
		}

		concurrentRA := perReaderCap / concurrentStreamReadaheadShareRatio
		if concurrentRA < minRA {
			concurrentRA = minRA
		}

		if concurrentRA < ra {
			ra = concurrentRA
		}

		if ra > perReaderCap {
			ra = perReaderCap
		}
	}

	if cacheCap > 0 && cacheCap < ra {
		return cacheCap
	}

	return ra
}

func bindStreamReaderContext(reader streamReaderContextSetter, req *http.Request) {
	if reader == nil || req == nil {
		return
	}

	reader.SetContext(req.Context())
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

func shouldWarmupPlaybackStartup(
	req *http.Request,
	startOffset int64,
	fileSize int64,
	state playbackStartupWarmupState,
) bool {
	return decidePlaybackStartupWarmup(req, startOffset, fileSize, state).eligible
}

func decidePlaybackStartupWarmup(
	req *http.Request,
	startOffset int64,
	fileSize int64,
	state playbackStartupWarmupState,
) playbackStartupWarmupDecision {
	if req == nil {
		return playbackStartupWarmupDecision{skipReason: "invalid_request"}
	}

	if req.Method != http.MethodGet {
		return playbackStartupWarmupDecision{skipReason: "not_get"}
	}

	if !isPlaybackStreamRequest(req) {
		return playbackStartupWarmupDecision{skipReason: "not_playback"}
	}

	if startOffset < 0 || startOffset > startupWarmupMaxInitialOffset {
		return playbackStartupWarmupDecision{skipReason: "outside_startup_window"}
	}

	targetBytes := playbackStartupWarmupTargetBytes(fileSize, startOffset, state.cacheCapacityBytes)
	if targetBytes <= 0 {
		return playbackStartupWarmupDecision{skipReason: "no_target"}
	}

	if state.preloadTargetBytes >= targetBytes && state.preloadedBytes >= targetBytes {
		return playbackStartupWarmupDecision{targetBytes: targetBytes, skipReason: "preload_satisfied"}
	}

	return playbackStartupWarmupDecision{eligible: true, targetBytes: targetBytes}
}

func isPlaybackStreamRequest(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}

	if _, ok := req.URL.Query()["play"]; ok {
		return true
	}

	path := req.URL.Path

	return path == "/streams/play" || strings.HasPrefix(path, "/play/")
}

func playbackStartupWarmupTargetBytes(fileSize, startOffset, cacheCapacity int64) int64 {
	remaining := fileSize - startOffset
	if remaining <= 0 {
		return 0
	}

	target := startupWarmupDefaultMaxBytes
	cacheShareRatio := startupWarmupCacheShareRatio

	if cacheCapacity > 0 {
		if fileSize >= startupWarmupHeavyFileThreshold {
			target = startupWarmupHeavyFileMaxBytes
			cacheShareRatio = startupWarmupHeavyCacheShareRatio
		}

		cacheTarget := cacheCapacity / cacheShareRatio
		if cacheTarget <= 0 {
			cacheTarget = cacheCapacity
		}

		if cacheTarget < target {
			target = cacheTarget
		}
	}

	if target <= 0 || target > startupWarmupHeavyFileMaxBytes {
		target = startupWarmupDefaultMaxBytes
	}

	if target > remaining {
		target = remaining
	}

	return target
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

func (t *Torrent) streamFileForRequest(
	fileID int,
	maxSize int64,
	req *http.Request,
	resp http.ResponseWriter,
) (*torrent.File, error) {
	if !t.GotInfo() {
		http.NotFound(resp, req)

		return nil, errors.New("torrent doesn't have info yet")
	}

	file, err := findFileByID(t, fileID)
	if err != nil {
		return nil, err
	}

	if maxSize > 0 && file.Length() > maxSize {
		log.TLogln("File size exceeded:", file.DisplayPath(), file.Length(), "max:", maxSize)
		http.Error(resp, fmt.Sprintf("file size exceeded max allowed %d bytes", maxSize), http.StatusForbidden)

		return nil, fmt.Errorf("file size exceeded max allowed %d bytes", maxSize)
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

	resp.Header().Set("Connection", "close")
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

	bindStreamReaderContext(reader, req)

	activeReaders := 1
	if t.cache != nil {
		activeReaders = t.cache.GetUseReaders()
	}

	playbackTorrents := estimatePlaybackTorrents(GetActiveStreams(), activeReaders)
	if t.bt != nil {
		playbackTorrents = t.bt.ActivePlaybackTorrents()
	}

	readahead := defaultStreamReaderReadahead
	if t.Info() != nil {
		readahead = streamReaderReadahead(t.Info().PieceLength, 0, activeReaders)
	}

	if t.cache != nil {
		readahead = initialPlaybackReaderReadahead(
			0,
			t.cache.GetCapacity(),
			activeReaders,
			playbackTorrents,
			curSets.StreamConfig(),
		)
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

func (t *Torrent) warmupPlaybackStartup(
	req *http.Request,
	reader playbackStartupWarmupReader,
	fileSize int64,
	debugEnabled bool,
	delivery *streamDelivery,
) error {
	if reader == nil {
		if delivery != nil {
			delivery.startup.recordSkipped("reader_unavailable", 0)
		}

		return nil
	}

	startOffset := reader.Offset()
	state := t.playbackStartupWarmupState()
	decision := decidePlaybackStartupWarmup(req, startOffset, fileSize, state)

	if !decision.eligible {
		if delivery != nil {
			delivery.startup.recordSkipped(decision.skipReason, decision.targetBytes)
		}

		return nil
	}

	if delivery != nil {
		delivery.startup.recordWarmupStarted(decision.targetBytes)
	}

	result, err := runPlaybackStartupWarmupReader(
		req,
		reader,
		startOffset,
		decision.targetBytes,
		debugEnabled,
	)
	if delivery != nil {
		delivery.startup.recordWarmupCompleted(result)
	}

	return err
}

func (t *Torrent) playbackStartupWarmupState() playbackStartupWarmupState {
	state := playbackStartupWarmupState{}
	if t == nil {
		return state
	}

	t.muTorrent.Lock()
	state.preloadTargetBytes = t.preload.targetBytes
	state.preloadedBytes = t.preload.loadedBytes
	t.muTorrent.Unlock()

	if t.cache != nil {
		state.cacheCapacityBytes = t.cache.GetCapacity()
		if filled := t.cache.Filled(); filled > state.preloadedBytes {
			state.preloadedBytes = filled
		}
	}

	return state
}

func warmupPlaybackStartupReader(
	req *http.Request,
	reader playbackStartupWarmupReader,
	startOffset int64,
	targetBytes int64,
	debugEnabled bool,
) error {
	_, err := runPlaybackStartupWarmupReader(req, reader, startOffset, targetBytes, debugEnabled)

	return err
}

func runPlaybackStartupWarmupReader(
	req *http.Request,
	reader playbackStartupWarmupReader,
	startOffset int64,
	targetBytes int64,
	debugEnabled bool,
) (playbackStartupWarmupResult, error) {
	result := playbackStartupWarmupResult{outcome: "skipped"}
	if req == nil || reader == nil || targetBytes <= 0 {
		return result, nil
	}

	started := time.Now()
	result.outcome = "running"
	reqCtx := req.Context()

	warmupCtx, cancel := context.WithTimeout(reqCtx, startupWarmupTimeout)
	defer cancel()

	reader.SetContext(warmupCtx)
	readBytes, readErr := readPlaybackStartupWarmup(reader, targetBytes)
	reader.SetContext(reqCtx)

	result.readBytes = readBytes

	if _, err := reader.Seek(startOffset, io.SeekStart); err != nil {
		result.elapsed = time.Since(started)
		result.outcome = "restore_failed"

		return result, fmt.Errorf("restore stream reader after startup warmup: %w", err)
	}

	result.elapsed = time.Since(started)
	result.offsetRestored = reader.Offset() == startOffset

	if debugEnabled {
		log.Debug(
			"stream startup warmup",
			"target_bytes", targetBytes,
			"read_bytes", readBytes,
			"elapsed_ms", result.elapsed.Milliseconds(),
			"error", readErr,
		)
	}

	switch {
	case reqCtx.Err() != nil:
		result.outcome = "request_canceled"

		return result, reqCtx.Err()
	case errors.Is(readErr, context.DeadlineExceeded):
		result.outcome = "timeout"

		return result, nil
	case errors.Is(readErr, io.EOF):
		result.outcome = "eof"

		return result, nil
	case readErr == nil && readBytes >= targetBytes:
		result.outcome = "success"

		return result, nil
	case readErr == nil:
		result.outcome = "short_read"

		return result, nil
	default:
		result.outcome = "read_error"

		return result, fmt.Errorf("stream startup warmup: %w", readErr)
	}
}

func readPlaybackStartupWarmup(reader io.Reader, targetBytes int64) (int64, error) {
	buf := make([]byte, streamCopyBufferSize)

	var readBytes int64

	for readBytes < targetBytes {
		readSize := len(buf)
		if remaining := targetBytes - readBytes; remaining < int64(readSize) {
			readSize = int(remaining)
		}

		n, err := reader.Read(buf[:readSize])
		if n > 0 {
			readBytes += int64(n)
		}

		if err != nil {
			return readBytes, err
		}

		if n == 0 {
			return readBytes, nil
		}
	}

	return readBytes, nil
}
