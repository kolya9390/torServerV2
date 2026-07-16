package torr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type errorStartupWarmupReader struct {
	offset   int64
	readErr  error
	contexts []context.Context
}

type generatedStartupWarmupReader struct {
	offset    int64
	size      int64
	maxOffset int64
	contexts  []context.Context
}

func (r *generatedStartupWarmupReader) Read(p []byte) (int, error) {
	if r.offset >= r.size {
		return 0, io.EOF
	}

	readBytes := min(int64(len(p)), r.size-r.offset)
	r.offset += readBytes
	if r.offset > r.maxOffset {
		r.maxOffset = r.offset
	}

	return int(readBytes), nil
}

func (r *generatedStartupWarmupReader) Seek(offset int64, whence int) (int64, error) {
	if whence != io.SeekStart || offset < 0 || offset > r.size {
		return r.offset, io.ErrUnexpectedEOF
	}

	r.offset = offset

	return r.offset, nil
}

func (r *generatedStartupWarmupReader) Offset() int64 {
	return r.offset
}

func (r *generatedStartupWarmupReader) SetContext(ctx context.Context) {
	r.contexts = append(r.contexts, ctx)
}

func (r *errorStartupWarmupReader) Read([]byte) (int, error) {
	return 0, r.readErr
}

func (r *errorStartupWarmupReader) Seek(offset int64, whence int) (int64, error) {
	if whence != io.SeekStart {
		return r.offset, io.ErrUnexpectedEOF
	}

	r.offset = offset

	return r.offset, nil
}

func (r *errorStartupWarmupReader) Offset() int64 {
	return r.offset
}

func (r *errorStartupWarmupReader) SetContext(ctx context.Context) {
	r.contexts = append(r.contexts, ctx)
}

func TestPlaybackStartupWarmupDecisionReportsSkipReason(t *testing.T) {
	state := playbackStartupWarmupState{
		preloadTargetBytes: 8 << 20,
		preloadedBytes:     8 << 20,
		cacheCapacityBytes: 64 << 20,
	}
	req := httptest.NewRequest(http.MethodGet, "/stream/movie.mkv?play", nil)

	decision := decidePlaybackStartupWarmup(req, 0, 128<<20, state)
	if decision.eligible {
		t.Fatal("decision.eligible = true, want false")
	}

	if got, want := decision.skipReason, "preload_satisfied"; got != want {
		t.Fatalf("decision.skipReason = %q, want %q", got, want)
	}

	if got, want := decision.targetBytes, int64(8<<20); got != want {
		t.Fatalf("decision.targetBytes = %d, want %d", got, want)
	}
}

func TestRunPlaybackStartupWarmupReaderReportsSuccessAndRestoresOffset(t *testing.T) {
	reader := &fakeStartupWarmupReader{
		fakeStreamContentSource: fakeStreamContentSource{
			data: []byte("0123456789abcdefghijklmnopqrstuvwxyz"),
			pos:  5,
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/stream/movie.mkv?play", nil)

	result, err := runPlaybackStartupWarmupReader(req, reader, 5, 8, false)
	if err != nil {
		t.Fatalf("runPlaybackStartupWarmupReader() err = %v", err)
	}

	if got, want := result.outcome, "success"; got != want {
		t.Fatalf("result.outcome = %q, want %q", got, want)
	}

	if got, want := result.readBytes, int64(8); got != want {
		t.Fatalf("result.readBytes = %d, want %d", got, want)
	}

	if !result.offsetRestored {
		t.Fatal("result.offsetRestored = false, want true")
	}
}

func TestPlaybackStartupWarmupCoversA208HeavyFileStarvationFrontier(t *testing.T) {
	const (
		fileSize             = int64(100 << 30)
		cacheCapacity        = int64(64 << 20)
		observedWaitFrontier = int64(24 << 20)
	)

	targetBytes := playbackStartupWarmupTargetBytes(fileSize, 0, cacheCapacity)
	if targetBytes <= observedWaitFrontier {
		t.Fatalf("target bytes = %d, want beyond observed %d-byte wait frontier", targetBytes, observedWaitFrontier)
	}

	reader := &generatedStartupWarmupReader{size: fileSize}
	req := httptest.NewRequest(http.MethodGet, "/stream/movie.mkv?play", nil)

	result, err := runPlaybackStartupWarmupReader(req, reader, 0, targetBytes, false)
	if err != nil {
		t.Fatalf("runPlaybackStartupWarmupReader() err = %v", err)
	}

	if got, want := result.outcome, "success"; got != want {
		t.Fatalf("result.outcome = %q, want %q", got, want)
	}

	if got := reader.maxOffset; got < targetBytes {
		t.Fatalf("maximum warmed offset = %d, want at least %d", got, targetBytes)
	}

	if got := reader.Offset(); got != 0 {
		t.Fatalf("reader offset after warmup = %d, want 0", got)
	}
}

func TestRunPlaybackStartupWarmupReaderClassifiesTimeoutAndCancellation(t *testing.T) {
	tests := []struct {
		name        string
		request     func() *http.Request
		readErr     error
		wantOutcome string
		wantError   bool
	}{
		{
			name: "timeout",
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/stream/movie.mkv?play", nil)
			},
			readErr:     context.DeadlineExceeded,
			wantOutcome: "timeout",
		},
		{
			name: "request cancellation",
			request: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/stream/movie.mkv?play", nil)
				ctx, cancel := context.WithCancel(req.Context())
				cancel()

				return req.WithContext(ctx)
			},
			readErr:     context.Canceled,
			wantOutcome: "request_canceled",
			wantError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &errorStartupWarmupReader{offset: 7, readErr: tt.readErr}
			req := tt.request()
			result, err := runPlaybackStartupWarmupReader(req, reader, 7, 1024, false)

			if (err != nil) != tt.wantError {
				t.Fatalf("runPlaybackStartupWarmupReader() err = %v, wantError %v", err, tt.wantError)
			}

			if got := result.outcome; got != tt.wantOutcome {
				t.Fatalf("result.outcome = %q, want %q", got, tt.wantOutcome)
			}

			if !result.offsetRestored {
				t.Fatal("result.offsetRestored = false, want true")
			}

			if got, want := len(reader.contexts), 2; got != want {
				t.Fatalf("reader contexts = %d, want %d", got, want)
			}

			if _, ok := reader.contexts[0].Deadline(); !ok {
				t.Fatal("warmup context has no deadline")
			}

			if reader.contexts[1] != req.Context() {
				t.Fatal("request context was not restored after warmup")
			}
		})
	}
}

func TestRunPlaybackStartupWarmupReaderConcurrentReadersRemainIndependent(t *testing.T) {
	const (
		readerCount = 16
		targetBytes = int64(1 << 20)
	)

	type concurrentResult struct {
		index  int
		reader *generatedStartupWarmupReader
		result playbackStartupWarmupResult
		err    error
	}

	results := make(chan concurrentResult, readerCount)

	for index := range readerCount {
		go func() {
			startOffset := int64(index * 4096)
			reader := &generatedStartupWarmupReader{
				offset: startOffset,
				size:   startOffset + targetBytes,
			}
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/stream/%d?play", index), nil)
			result, err := runPlaybackStartupWarmupReader(req, reader, startOffset, targetBytes, false)
			results <- concurrentResult{index: index, reader: reader, result: result, err: err}
		}()
	}

	for range readerCount {
		item := <-results
		if item.err != nil {
			t.Errorf("reader %d warmup err = %v", item.index, item.err)

			continue
		}

		if got, want := item.result.outcome, "success"; got != want {
			t.Errorf("reader %d outcome = %q, want %q", item.index, got, want)
		}

		if got, want := item.reader.Offset(), int64(item.index*4096); got != want {
			t.Errorf("reader %d offset = %d, want %d", item.index, got, want)
		}

		if got, want := len(item.reader.contexts), 2; got != want {
			t.Errorf("reader %d contexts = %d, want %d", item.index, got, want)
		}
	}
}

func TestStreamStartupTimelineCapturesStatesAndFirstReadWait(t *testing.T) {
	resetStreamDeliveryForTest()

	state := streamStartupCacheSnapshot{
		CacheCapacityBytes: 64 << 20,
		ReaderOffset:       0,
		ReaderReadahead:    16 << 20,
	}
	delivery, release := registerStreamDeliveryWithMetadata(time.Now(), 42, streamDeliveryMetadata{
		fileSize: 128 << 20,
		startupState: func() streamStartupCacheSnapshot {
			return state
		},
	})
	defer release()

	delivery.startup.recordWarmupStarted(8 << 20)

	state.CacheFilledBytes = 8 << 20
	state.ResidentPieces = 4

	delivery.startup.recordWarmupCompleted(playbackStartupWarmupResult{
		readBytes:      8 << 20,
		elapsed:        250 * time.Millisecond,
		outcome:        "success",
		offsetRestored: true,
	})

	state.CacheFilledBytes = 12 << 20
	state.ResidentPieces = 6
	state.ReaderOffset = 8 << 20

	delivery.recordReadWait(750*time.Millisecond, 8<<20, 32<<10)

	state.ReaderOffset = 16 << 20

	delivery.recordReadWait(time.Second, 16<<20, 64<<10)

	stream := SnapshotStreamDelivery().Streams[0]
	startup := stream.Startup
	if !startup.WarmupEligible || startup.Outcome != "success" || !startup.OffsetRestored {
		t.Fatalf("startup = %+v, want completed eligible warmup", startup)
	}

	if got, want := startup.AtCompletion.CacheFilledBytes, int64(8<<20); got != want {
		t.Fatalf("completion cache fill = %d, want %d", got, want)
	}

	if startup.FirstReadWait == nil {
		t.Fatal("FirstReadWait = nil, want event")
	}

	if got, want := startup.FirstReadWait.Offset, int64(8<<20); got != want {
		t.Fatalf("first wait offset = %d, want %d", got, want)
	}

	if got, want := startup.FirstReadWait.State.ResidentPieces, int64(6); got != want {
		t.Fatalf("first wait resident pieces = %d, want %d", got, want)
	}
}

func TestStreamStartupTimelineIsBoundedCleanedUpAndPrivacySafe(t *testing.T) {
	resetStreamDeliveryForTest()

	releases := make([]func(), 0, maxStreamDeliverySnapshotSize+5)

	for range maxStreamDeliverySnapshotSize + 5 {
		delivery, release := registerStreamDeliveryWithMetadata(time.Now(), 1, streamDeliveryMetadata{
			startupState: func() streamStartupCacheSnapshot {
				return streamStartupCacheSnapshot{CacheCapacityBytes: 64 << 20}
			},
		})
		delivery.startup.recordSkipped("preload_satisfied", 8<<20)

		releases = append(releases, release)
	}

	snapshot := SnapshotStreamDelivery()
	if got, want := snapshot.ActiveStreams, maxStreamDeliverySnapshotSize+5; got != want {
		t.Fatalf("ActiveStreams = %d, want %d", got, want)
	}

	if got, want := len(snapshot.Streams), maxStreamDeliverySnapshotSize; got != want {
		t.Fatalf("len(Streams) = %d, want %d", got, want)
	}

	if got, want := snapshot.Streams[0].Startup.Outcome, "skipped"; got != want {
		t.Fatalf("startup outcome = %q, want %q", got, want)
	}

	if got, want := snapshot.Streams[0].Startup.SkipReason, "preload_satisfied"; got != want {
		t.Fatalf("startup skip reason = %q, want %q", got, want)
	}

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("json.Marshal() err = %v", err)
	}

	payload := strings.ToLower(string(raw))
	for _, forbidden := range []string{"magnet", "title", "path", "query", "remote", "peer_address"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("startup snapshot leaks %q: %s", forbidden, payload)
		}
	}

	for _, release := range releases {
		release()
	}

	if got := SnapshotStreamDelivery().ActiveStreams; got != 0 {
		t.Fatalf("ActiveStreams after cleanup = %d, want 0", got)
	}
}
