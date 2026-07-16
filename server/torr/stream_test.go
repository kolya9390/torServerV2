package torr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"server/settings"
)

type fakeStreamContentSource struct {
	data  []byte
	pos   int64
	seeks [][2]int64
}

type slowOnceReader struct {
	delay time.Duration
	data  []byte
	done  bool
}

type slowOffsetReader struct {
	delay  time.Duration
	data   []byte
	offset int64
	done   bool
}

type bufferSizeReader struct {
	maxReadSize int
	done        bool
}

type fakeContextReader struct {
	ctx context.Context
}

type fakeStartupWarmupReader struct {
	fakeStreamContentSource
	contexts []context.Context
}

func (r *fakeContextReader) SetContext(ctx context.Context) {
	r.ctx = ctx
}

func (r *fakeStartupWarmupReader) SetContext(ctx context.Context) {
	r.contexts = append(r.contexts, ctx)
}

func (r *slowOnceReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}

	time.Sleep(r.delay)
	r.done = true

	return copy(p, r.data), nil
}

func (r *slowOffsetReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}

	time.Sleep(r.delay)
	r.done = true
	n := copy(p, r.data)
	r.offset += int64(n)

	return n, nil
}

func (r *slowOffsetReader) Offset() int64 {
	return r.offset
}

func (r *bufferSizeReader) Read(p []byte) (int, error) {
	if len(p) > r.maxReadSize {
		r.maxReadSize = len(p)
	}

	if r.done {
		return 0, io.EOF
	}

	r.done = true
	p[0] = 'x'

	return 1, nil
}

func (f *fakeStreamContentSource) Read(p []byte) (int, error) {
	if f.pos >= int64(len(f.data)) {
		return 0, io.EOF
	}

	n := copy(p, f.data[f.pos:])
	f.pos += int64(n)

	return n, nil
}

func (f *fakeStreamContentSource) Seek(offset int64, whence int) (int64, error) {
	var base int64

	switch whence {
	case io.SeekStart:
		base = 0
	case io.SeekCurrent:
		base = f.pos
	case io.SeekEnd:
		base = int64(len(f.data))
	default:
		return 0, io.ErrUnexpectedEOF
	}

	target := base + offset
	if target < 0 {
		return 0, io.ErrUnexpectedEOF
	}

	f.pos = target
	f.seeks = append(f.seeks, [2]int64{offset, int64(whence)})

	return f.pos, nil
}

func (f *fakeStreamContentSource) Offset() int64 {
	return f.pos
}

func TestServeContentReadSeeker_SkipsUnderlyingSizeProbe(t *testing.T) {
	src := &fakeStreamContentSource{data: []byte("0123456789")}
	wrapped := newServeContentReadSeeker(src, int64(len(src.data)), nil)

	if pos, err := wrapped.Seek(0, io.SeekEnd); err != nil || pos != int64(len(src.data)) {
		t.Fatalf("SeekEnd() = (%d, %v), want (%d, nil)", pos, err, len(src.data))
	}

	if pos, err := wrapped.Seek(0, io.SeekStart); err != nil || pos != 0 {
		t.Fatalf("SeekStart() after probe = (%d, %v), want (0, nil)", pos, err)
	}

	if got := len(src.seeks); got != 0 {
		t.Fatalf("underlying seek count after size probe = %d, want 0", got)
	}

	buf := make([]byte, 4)
	n, err := wrapped.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read() err = %v", err)
	}

	if got, want := string(buf[:n]), "0123"; got != want {
		t.Fatalf("Read() = %q, want %q", got, want)
	}

	if got := len(src.seeks); got != 1 {
		t.Fatalf("underlying seek count after first real read = %d, want 1", got)
	}
}

func TestServeContentReadSeeker_DefersRangeSeekUntilRead(t *testing.T) {
	src := &fakeStreamContentSource{data: []byte("abcdefghijklmnopqrstuvwxyz")}
	wrapped := newServeContentReadSeeker(src, int64(len(src.data)), nil)

	if _, err := wrapped.Seek(0, io.SeekEnd); err != nil {
		t.Fatalf("SeekEnd() err = %v", err)
	}

	if pos, err := wrapped.Seek(10, io.SeekStart); err != nil || pos != 10 {
		t.Fatalf("SeekStart(10) = (%d, %v), want (10, nil)", pos, err)
	}

	if got := len(src.seeks); got != 0 {
		t.Fatalf("underlying seek count before read = %d, want 0", got)
	}

	buf := make([]byte, 5)
	n, err := wrapped.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read() err = %v", err)
	}

	if got, want := string(buf[:n]), "klmno"; got != want {
		t.Fatalf("Read() = %q, want %q", got, want)
	}

	if got, want := src.seeks, [][2]int64{{10, int64(io.SeekStart)}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("underlying seeks = %v, want %v", got, want)
	}
}

func TestServeContentReadSeekerRecordsReadWaitLocation(t *testing.T) {
	resetStreamDeliveryForTest()

	src := &fakeStreamContentSource{data: []byte("abcdef"), pos: 2}
	delivery, release := registerStreamDelivery(time.Now(), 77)
	defer release()

	wrapped := newServeContentReadSeeker(src, int64(len(src.data)), delivery)
	wrapped.recordReadWaitLocation(streamReadWaitThreshold+time.Millisecond, wrapped.Offset(), 32768)

	snapshot := SnapshotStreamDelivery()
	if got, want := len(snapshot.Streams), 1; got != want {
		t.Fatalf("len(Streams) = %d, want %d", got, want)
	}

	stream := snapshot.Streams[0]
	if got, want := stream.LastReadOffset, int64(2); got != want {
		t.Fatalf("LastReadOffset = %d, want %d", got, want)
	}

	if got, want := stream.LastReadSize, int64(32768); got != want {
		t.Fatalf("LastReadSize = %d, want %d", got, want)
	}
}

func TestStreamReaderReadahead(t *testing.T) {
	tests := []struct {
		name      string
		pieceLen  int64
		cacheCap  int64
		readers   int
		wantBytes int64
	}{
		{name: "64mb cache one reader keeps fixed horizon", pieceLen: 2 << 20, cacheCap: 64 << 20, readers: 1, wantBytes: 16 << 20},
		{name: "64mb cache two readers narrows cache share", pieceLen: 2 << 20, cacheCap: 64 << 20, readers: 2, wantBytes: 8 << 20},
		{name: "64mb cache three readers divides cache share", pieceLen: 2 << 20, cacheCap: 64 << 20, readers: 3, wantBytes: ((64 << 20) / 3) / 4},
		{name: "128mb cache one reader keeps fixed horizon", pieceLen: 2 << 20, cacheCap: 128 << 20, readers: 1, wantBytes: 16 << 20},
		{name: "128mb cache two readers keep fixed horizon", pieceLen: 2 << 20, cacheCap: 128 << 20, readers: 2, wantBytes: 16 << 20},
		{name: "128mb cache three readers divides cache share", pieceLen: 2 << 20, cacheCap: 128 << 20, readers: 3, wantBytes: ((128 << 20) / 3) / 4},
		{name: "256mb cache one reader keeps fixed horizon", pieceLen: 2 << 20, cacheCap: 256 << 20, readers: 1, wantBytes: 16 << 20},
		{name: "256mb cache two readers keep fixed horizon", pieceLen: 2 << 20, cacheCap: 256 << 20, readers: 2, wantBytes: 16 << 20},
		{name: "256mb cache three readers keep fixed horizon", pieceLen: 2 << 20, cacheCap: 256 << 20, readers: 3, wantBytes: 16 << 20},
		{name: "falls back to fixed baseline", pieceLen: 4 << 20, cacheCap: 0, readers: 1, wantBytes: 16 << 20},
		{name: "respects tiny cache", pieceLen: 1 << 20, cacheCap: 6 << 20, readers: 1, wantBytes: 6 << 20},
		{name: "keeps at least two large pieces", pieceLen: 4 << 20, cacheCap: 64 << 20, readers: 3, wantBytes: 8 << 20},
		{name: "does not exceed per reader capacity", pieceLen: 1 << 20, cacheCap: 8 << 20, readers: 4, wantBytes: 2 << 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := streamReaderReadahead(tt.pieceLen, tt.cacheCap, tt.readers); got != tt.wantBytes {
				t.Fatalf(
					"streamReaderReadahead(%d, %d, %d) = %d, want %d",
					tt.pieceLen,
					tt.cacheCap,
					tt.readers,
					got,
					tt.wantBytes,
				)
			}
		})
	}
}

func TestInitialPlaybackReaderReadaheadHonorsAdaptiveLimitForHTTPReader(t *testing.T) {
	got := initialPlaybackReaderReadahead(
		2<<20,
		256<<20,
		1,
		1,
		settings.StreamConfig{
			AdaptiveRAMinMB: 4,
			AdaptiveRAMaxMB: 128,
		},
	)

	if want := int64(128 << 20); got != want {
		t.Fatalf("initialPlaybackReaderReadahead() = %d, want %d", got, want)
	}
}

func TestInitialPlaybackReaderReadaheadBoundsPerReaderCapacity(t *testing.T) {
	got := initialPlaybackReaderReadahead(
		2<<20,
		256<<20,
		3,
		1,
		settings.StreamConfig{
			AdaptiveRAMinMB: 4,
			AdaptiveRAMaxMB: 128,
		},
	)

	if want := int64((256 << 20) / 3); got != want {
		t.Fatalf("initialPlaybackReaderReadahead() = %d, want %d", got, want)
	}
}

func TestInitialPlaybackReaderReadaheadScalesDownAboveTwoPlaybackTorrents(t *testing.T) {
	got := initialPlaybackReaderReadahead(
		2<<20,
		256<<20,
		1,
		4,
		settings.StreamConfig{
			AdaptiveRAMinMB: 4,
			AdaptiveRAMaxMB: 128,
		},
	)

	if want := int64(64 << 20); got != want {
		t.Fatalf("initialPlaybackReaderReadahead() = %d, want %d", got, want)
	}
}

func TestInitialPlaybackReaderReadaheadUsesLargeSingleStreamWindow(t *testing.T) {
	got := initialPlaybackReaderReadahead(
		2<<20,
		1024<<20,
		1,
		1,
		settings.StreamConfig{
			AdaptiveRAMinMB: 4,
			AdaptiveRAMaxMB: 512,
		},
	)

	if want := int64(512 << 20); got != want {
		t.Fatalf("initialPlaybackReaderReadahead() = %d, want %d", got, want)
	}
}

func TestBindStreamReaderContext_UsesRequestContext(t *testing.T) {
	reader := &fakeContextReader{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/stream/movie.mkv", nil).WithContext(ctx)

	bindStreamReaderContext(reader, req)

	if reader.ctx != ctx {
		t.Fatalf("reader context = %v, want request context %v", reader.ctx, ctx)
	}
}

func TestBindStreamReaderContext_NilSafe(t *testing.T) {
	bindStreamReaderContext(nil, nil)
	bindStreamReaderContext(&fakeContextReader{}, nil)
}

func TestShouldWarmupPlaybackStartup(t *testing.T) {
	tests := []struct {
		name        string
		requestPath string
		method      string
		startOffset int64
		fileSize    int64
		state       playbackStartupWarmupState
		want        bool
	}{
		{
			name:        "legacy play without preload warms initial startup",
			requestPath: "/stream/movie.mkv?link=abc&play",
			method:      http.MethodGet,
			fileSize:    128 << 20,
			state:       playbackStartupWarmupState{cacheCapacityBytes: 64 << 20},
			want:        true,
		},
		{
			name:        "streams play warms initial startup",
			requestPath: "/streams/play?link=abc",
			method:      http.MethodGet,
			fileSize:    128 << 20,
			state:       playbackStartupWarmupState{cacheCapacityBytes: 64 << 20},
			want:        true,
		},
		{
			name:        "play route warms initial startup",
			requestPath: "/play/hash/1",
			method:      http.MethodGet,
			fileSize:    128 << 20,
			state:       playbackStartupWarmupState{cacheCapacityBytes: 64 << 20},
			want:        true,
		},
		{
			name:        "completed preload skips startup warmup",
			requestPath: "/stream/movie.mkv?link=abc&play",
			method:      http.MethodGet,
			fileSize:    128 << 20,
			state: playbackStartupWarmupState{
				preloadTargetBytes: 8 << 20,
				preloadedBytes:     8 << 20,
				cacheCapacityBytes: 64 << 20,
			},
		},
		{
			name:        "far range skip avoids seek warmup",
			requestPath: "/stream/movie.mkv?link=abc&play",
			method:      http.MethodGet,
			startOffset: startupWarmupMaxInitialOffset + 1,
			fileSize:    128 << 20,
			state:       playbackStartupWarmupState{cacheCapacityBytes: 64 << 20},
		},
		{
			name:        "head request skip",
			requestPath: "/stream/movie.mkv?link=abc&play",
			method:      http.MethodHead,
			fileSize:    128 << 20,
			state:       playbackStartupWarmupState{cacheCapacityBytes: 64 << 20},
		},
		{
			name:        "non playback request skip",
			requestPath: "/stream/movie.mkv?link=abc",
			method:      http.MethodGet,
			fileSize:    128 << 20,
			state:       playbackStartupWarmupState{cacheCapacityBytes: 64 << 20},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.requestPath, nil)
			got := shouldWarmupPlaybackStartup(req, tt.startOffset, tt.fileSize, tt.state)
			if got != tt.want {
				t.Fatalf("shouldWarmupPlaybackStartup() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlaybackStartupWarmupTargetBytes(t *testing.T) {
	tests := []struct {
		name        string
		fileSize    int64
		startOffset int64
		cacheCap    int64
		want        int64
	}{
		{name: "caps to max warmup bytes", fileSize: 128 << 20, cacheCap: 256 << 20, want: 8 << 20},
		{name: "uses cache fraction for small cache", fileSize: 128 << 20, cacheCap: 32 << 20, want: 4 << 20},
		{name: "falls back to max without cache", fileSize: 128 << 20, want: 8 << 20},
		{name: "uses bounded heavy warmup for release cache", fileSize: 100 << 30, cacheCap: 64 << 20, want: 32 << 20},
		{name: "caps heavy warmup for intermediate cache", fileSize: 100 << 30, cacheCap: 128 << 20, want: 32 << 20},
		{name: "caps heavy file warmup for home 4k cache", fileSize: 100 << 30, cacheCap: 512 << 20, want: 32 << 20},
		{name: "uses half of a small cache for heavy file", fileSize: 100 << 30, cacheCap: 32 << 20, want: 16 << 20},
		{name: "caps to remaining file bytes", fileSize: 10 << 20, startOffset: 6 << 20, cacheCap: 256 << 20, want: 4 << 20},
		{name: "caps heavy file warmup to remaining bytes", fileSize: 100 << 30, startOffset: 100<<30 - 20<<20, cacheCap: 512 << 20, want: 20 << 20},
		{name: "zero when offset reaches end", fileSize: 10 << 20, startOffset: 10 << 20, cacheCap: 256 << 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := playbackStartupWarmupTargetBytes(tt.fileSize, tt.startOffset, tt.cacheCap)
			if got != tt.want {
				t.Fatalf("playbackStartupWarmupTargetBytes() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWarmupPlaybackStartupReaderRestoresOffset(t *testing.T) {
	reader := &fakeStartupWarmupReader{
		fakeStreamContentSource: fakeStreamContentSource{
			data: []byte("0123456789abcdefghijklmnopqrstuvwxyz"),
			pos:  5,
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/stream/movie.mkv?play", nil)

	err := warmupPlaybackStartupReader(req, reader, 5, 8, false)
	if err != nil {
		t.Fatalf("warmupPlaybackStartupReader() err = %v", err)
	}

	if got, want := reader.Offset(), int64(5); got != want {
		t.Fatalf("reader offset after warmup = %d, want %d", got, want)
	}

	if got, want := len(reader.contexts), 2; got != want {
		t.Fatalf("reader contexts = %d, want %d", got, want)
	}

	if got, want := reader.seeks, [][2]int64{{5, int64(io.SeekStart)}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("reader seeks = %v, want %v", got, want)
	}
}

type readerFromRecorder struct {
	header http.Header
	body   bytes.Buffer
	status int
}

type writeOnlyRecorder struct {
	header http.Header
	body   bytes.Buffer
	status int
}

type slowResponseWriter struct {
	header http.Header
	delay  time.Duration
}

func (w *slowResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}

	return w.header
}

func (*slowResponseWriter) WriteHeader(int) {}

func (w *slowResponseWriter) Write(p []byte) (int, error) {
	time.Sleep(w.delay)

	return len(p), nil
}

func (r *readerFromRecorder) Header() http.Header {
	if r.header == nil {
		r.header = make(http.Header)
	}

	return r.header
}

func (r *readerFromRecorder) WriteHeader(status int) {
	r.status = status
}

func (r *readerFromRecorder) Write(p []byte) (int, error) {
	return r.body.Write(p)
}

func (r *readerFromRecorder) ReadFrom(src io.Reader) (int64, error) {
	return r.body.ReadFrom(src)
}

func (r *writeOnlyRecorder) Header() http.Header {
	if r.header == nil {
		r.header = make(http.Header)
	}

	return r.header
}

func (r *writeOnlyRecorder) WriteHeader(status int) {
	r.status = status
}

func (r *writeOnlyRecorder) Write(p []byte) (int, error) {
	return r.body.Write(p)
}

func TestStreamMetricsWriter_Write(t *testing.T) {
	resetStreamHealthForTest()

	rec := httptest.NewRecorder()
	w := &streamMetricsWriter{ResponseWriter: rec, trackReadWait: true}

	n, err := w.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write() err = %v", err)
	}

	if got, want := n, 5; got != want {
		t.Fatalf("Write() = %d, want %d", got, want)
	}

	if got, want := w.bytesWritten.Load(), int64(5); got != want {
		t.Fatalf("bytesWritten = %d, want %d", got, want)
	}

	if got := w.firstWriteUnixNano.Load(); got == 0 {
		t.Fatal("firstWriteUnixNano was not recorded")
	}

	if got, want := SnapshotStreamHealth().BytesWrittenTotal, int64(5); got != want {
		t.Fatalf("BytesWrittenTotal = %d, want %d", got, want)
	}
}

func TestStreamMetricsWriter_ReadFrom(t *testing.T) {
	resetStreamHealthForTest()

	rec := &readerFromRecorder{}
	w := &streamMetricsWriter{ResponseWriter: rec, trackReadWait: true}

	n, err := w.ReadFrom(bytes.NewBufferString("stream-data"))
	if err != nil {
		t.Fatalf("ReadFrom() err = %v", err)
	}

	if got, want := n, int64(len("stream-data")); got != want {
		t.Fatalf("ReadFrom() = %d, want %d", got, want)
	}

	if got, want := w.bytesWritten.Load(), int64(len("stream-data")); got != want {
		t.Fatalf("bytesWritten = %d, want %d", got, want)
	}

	if got := w.firstWriteUnixNano.Load(); got == 0 {
		t.Fatal("firstWriteUnixNano was not recorded")
	}

	if got, want := SnapshotStreamHealth().BytesWrittenTotal, int64(len("stream-data")); got != want {
		t.Fatalf("BytesWrittenTotal = %d, want %d", got, want)
	}
}

func TestStreamMetricsWriter_ReadFromUsesLargeDebugCopyBuffer(t *testing.T) {
	t.Parallel()

	rec := &writeOnlyRecorder{}
	reader := &bufferSizeReader{}
	w := &streamMetricsWriter{ResponseWriter: rec, trackReadWait: true}

	if _, err := w.ReadFrom(reader); err != nil {
		t.Fatalf("ReadFrom() err = %v", err)
	}

	if got := reader.maxReadSize; got != streamCopyBufferSize {
		t.Fatalf("copy buffer size = %d, want %d", got, streamCopyBufferSize)
	}
}

func TestStreamMetricsWriter_ReadFromFallback(t *testing.T) {
	t.Parallel()

	rec := &writeOnlyRecorder{}
	w := &streamMetricsWriter{ResponseWriter: rec}

	n, err := w.ReadFrom(bytes.NewBufferString("stream-data"))
	if err != nil {
		t.Fatalf("ReadFrom() err = %v", err)
	}

	if got, want := n, int64(len("stream-data")); got != want {
		t.Fatalf("ReadFrom() = %d, want %d", got, want)
	}

	if got, want := w.bytesWritten.Load(), int64(len("stream-data")); got != want {
		t.Fatalf("bytesWritten = %d, want %d", got, want)
	}

	if got, want := rec.body.String(), "stream-data"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}

	if got := w.firstWriteUnixNano.Load(); got == 0 {
		t.Fatal("firstWriteUnixNano was not recorded")
	}
}

func TestStreamHealthRecordsFirstByteAndBytes(t *testing.T) {
	resetStreamHealthForTest()

	recordStreamBytesWritten(1024)
	recordStreamCompleted(150*time.Millisecond, 1024, nil)

	snapshot := SnapshotStreamHealth()

	if got, want := snapshot.RequestsTotal, int64(1); got != want {
		t.Fatalf("RequestsTotal = %d, want %d", got, want)
	}

	if got, want := snapshot.BytesWrittenTotal, int64(1024); got != want {
		t.Fatalf("BytesWrittenTotal = %d, want %d", got, want)
	}

	if got, want := snapshot.FirstByteMSBuckets["le_500"], int64(1); got != want {
		t.Fatalf("first byte le_500 bucket = %d, want %d", got, want)
	}

	if got := snapshot.SlowFirstByteTotal; got != 0 {
		t.Fatalf("SlowFirstByteTotal = %d, want 0", got)
	}
}

func TestStreamHealthRecordsSlowFirstByteAndZeroBytes(t *testing.T) {
	resetStreamHealthForTest()

	recordStreamCompleted(3*time.Second, 0, nil)

	snapshot := SnapshotStreamHealth()

	if got, want := snapshot.SlowFirstByteTotal, int64(1); got != want {
		t.Fatalf("SlowFirstByteTotal = %d, want %d", got, want)
	}

	if got, want := snapshot.ZeroByteResponsesTotal, int64(1); got != want {
		t.Fatalf("ZeroByteResponsesTotal = %d, want %d", got, want)
	}

	if got, want := snapshot.StallsTotal, int64(1); got != want {
		t.Fatalf("StallsTotal = %d, want %d", got, want)
	}
}

func TestStreamHealthRecordsReadWait(t *testing.T) {
	resetStreamHealthForTest()

	recordStreamReadWait(750 * time.Millisecond)

	snapshot := SnapshotStreamHealth()

	if got, want := snapshot.StallsTotal, int64(1); got != want {
		t.Fatalf("StallsTotal = %d, want %d", got, want)
	}

	if got, want := snapshot.ReadWaitMSBuckets["le_1000"], int64(1); got != want {
		t.Fatalf("read wait le_1000 bucket = %d, want %d", got, want)
	}

	if got, want := snapshot.ReadWaitsTotal, int64(1); got != want {
		t.Fatalf("ReadWaitsTotal = %d, want %d", got, want)
	}

	if got, want := snapshot.ReadWaitTotalMS, int64(750); got != want {
		t.Fatalf("ReadWaitTotalMS = %d, want %d", got, want)
	}

	if got, want := snapshot.MaxReadWaitMS, int64(750); got != want {
		t.Fatalf("MaxReadWaitMS = %d, want %d", got, want)
	}
}

func TestStreamHealthRecordsLongReadWaits(t *testing.T) {
	resetStreamHealthForTest()

	recordStreamReadWait(4 * time.Second)
	recordStreamReadWait(11 * time.Second)

	snapshot := SnapshotStreamHealth()

	if got, want := snapshot.ReadWaitsTotal, int64(2); got != want {
		t.Fatalf("ReadWaitsTotal = %d, want %d", got, want)
	}

	if got, want := snapshot.ReadWaitsOver3sTotal, int64(2); got != want {
		t.Fatalf("ReadWaitsOver3sTotal = %d, want %d", got, want)
	}

	if got, want := snapshot.ReadWaitsOver10sTotal, int64(1); got != want {
		t.Fatalf("ReadWaitsOver10sTotal = %d, want %d", got, want)
	}

	if got, want := snapshot.MaxReadWaitMS, int64(11000); got != want {
		t.Fatalf("MaxReadWaitMS = %d, want %d", got, want)
	}
}

func TestStreamMetricsWriterReadFromRecordsSlowReadWait(t *testing.T) {
	resetStreamHealthForTest()
	resetStreamDeliveryForTest()

	rec := &readerFromRecorder{}
	delivery, release := registerStreamDelivery(time.Now(), 42)
	defer release()

	w := &streamMetricsWriter{
		ResponseWriter: rec,
		trackReadWait:  true,
		delivery:       delivery,
	}
	reader := &slowOnceReader{delay: streamReadWaitThreshold + 50*time.Millisecond, data: []byte("stream-data")}

	if _, err := w.ReadFrom(reader); err != nil {
		t.Fatalf("ReadFrom() err = %v", err)
	}

	snapshot := SnapshotStreamHealth()

	if got, want := snapshot.ReadWaitMSBuckets["le_1000"], int64(1); got != want {
		t.Fatalf("read wait le_1000 bucket = %d, want %d", got, want)
	}

	deliverySnapshot := SnapshotStreamDelivery()
	if got, want := len(deliverySnapshot.Streams), 1; got != want {
		t.Fatalf("len(delivery streams) = %d, want %d", got, want)
	}

	stream := deliverySnapshot.Streams[0]
	if got, want := stream.TorrentID, uint64(42); got != want {
		t.Fatalf("TorrentID = %d, want %d", got, want)
	}

	if got, want := stream.ReadWaitsTotal, int64(1); got != want {
		t.Fatalf("stream ReadWaitsTotal = %d, want %d", got, want)
	}

	if stream.MaxReadWaitMS < streamReadWaitThreshold.Milliseconds() {
		t.Fatalf("stream MaxReadWaitMS = %d, want >= %d", stream.MaxReadWaitMS, streamReadWaitThreshold.Milliseconds())
	}
}

func TestStreamMetricsWriterReadFromRecordsReadWaitDiagnostics(t *testing.T) {
	resetStreamHealthForTest()
	resetStreamDeliveryForTest()

	rec := &readerFromRecorder{}
	delivery, release := registerStreamDelivery(time.Now(), 77)
	defer release()

	w := &streamMetricsWriter{
		ResponseWriter: rec,
		trackReadWait:  true,
		delivery:       delivery,
	}
	reader := &slowOffsetReader{
		delay:  streamReadWaitThreshold + 50*time.Millisecond,
		data:   []byte("stream-data"),
		offset: 12345,
	}

	if _, err := w.ReadFrom(reader); err != nil {
		t.Fatalf("ReadFrom() err = %v", err)
	}

	deliverySnapshot := SnapshotStreamDelivery()
	if got, want := len(deliverySnapshot.Streams), 1; got != want {
		t.Fatalf("len(delivery streams) = %d, want %d", got, want)
	}

	stream := deliverySnapshot.Streams[0]
	if got, want := stream.LastReadOffset, int64(12345); got != want {
		t.Fatalf("LastReadOffset = %d, want %d", got, want)
	}

	if stream.LastReadSize <= 0 {
		t.Fatalf("LastReadSize = %d, want positive", stream.LastReadSize)
	}

	if stream.LastReadWaitMS < streamReadWaitThreshold.Milliseconds() {
		t.Fatalf("LastReadWaitMS = %d, want >= %d", stream.LastReadWaitMS, streamReadWaitThreshold.Milliseconds())
	}

	if got, want := stream.ReadWaitMSBuckets["le_1000"], int64(1); got != want {
		t.Fatalf("stream read wait le_1000 bucket = %d, want %d", got, want)
	}
}

func TestStreamDeliveryReadWaitDiagnosticsSnapshotIsPrivacySafe(t *testing.T) {
	resetStreamDeliveryForTest()

	delivery, release := registerStreamDelivery(time.Now(), 77)
	defer release()

	delivery.recordReadWait(streamReadWaitThreshold+time.Millisecond, 12345, 32768)

	data, err := json.Marshal(SnapshotStreamDelivery())
	if err != nil {
		t.Fatalf("marshal delivery snapshot: %v", err)
	}

	payload := strings.ToLower(string(data))
	for _, forbidden := range []string{"hash", "title", "path", "url", "query", "ip"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("delivery snapshot leaks %q: %s", forbidden, payload)
		}
	}
}

func TestStreamDeliveryReadWaitKeepsKnownOffset(t *testing.T) {
	resetStreamDeliveryForTest()

	delivery, release := registerStreamDelivery(time.Now(), 77)
	defer release()

	delivery.recordReadWaitLocation(streamReadWaitThreshold+time.Millisecond, 12345, 262144)
	delivery.recordReadWait(streamReadWaitThreshold+time.Millisecond, -1, 262144)

	streams := SnapshotStreamDelivery().Streams
	if got, want := len(streams), 1; got != want {
		t.Fatalf("len(Streams) = %d, want %d", got, want)
	}

	if got, want := streams[0].LastReadOffset, int64(12345); got != want {
		t.Fatalf("LastReadOffset = %d, want %d", got, want)
	}
}

func TestStreamMetricsWriterReadFromRecordsReadWaitWithoutDelivery(t *testing.T) {
	resetStreamHealthForTest()
	resetStreamDeliveryForTest()

	rec := &readerFromRecorder{}
	w := &streamMetricsWriter{
		ResponseWriter: rec,
		trackReadWait:  true,
	}
	reader := &slowOnceReader{delay: streamReadWaitThreshold + 50*time.Millisecond, data: []byte("stream-data")}

	if _, err := w.ReadFrom(reader); err != nil {
		t.Fatalf("ReadFrom() err = %v", err)
	}

	snapshot := SnapshotStreamHealth()
	if got, want := snapshot.ReadWaitsTotal, int64(1); got != want {
		t.Fatalf("ReadWaitsTotal = %d, want %d", got, want)
	}

	deliverySnapshot := SnapshotStreamDelivery()
	if got := deliverySnapshot.ActiveStreams; got != 0 {
		t.Fatalf("delivery ActiveStreams = %d, want 0", got)
	}
}

func TestStreamMetricsWriterReadFromSkipsReadWaitWhenDebugDisabled(t *testing.T) {
	resetStreamHealthForTest()
	resetStreamDeliveryForTest()

	rec := &readerFromRecorder{}
	delivery, release := registerStreamDelivery(time.Now(), 42)
	defer release()

	w := &streamMetricsWriter{
		ResponseWriter: rec,
		delivery:       delivery,
	}
	reader := &slowOnceReader{delay: streamReadWaitThreshold + 50*time.Millisecond, data: []byte("stream-data")}

	if _, err := w.ReadFrom(reader); err != nil {
		t.Fatalf("ReadFrom() err = %v", err)
	}

	snapshot := SnapshotStreamHealth()
	if got := snapshot.StallsTotal; got != 0 {
		t.Fatalf("StallsTotal = %d, want 0", got)
	}

	if got := snapshot.ReadWaitsTotal; got != 0 {
		t.Fatalf("ReadWaitsTotal = %d, want 0", got)
	}

	if got := snapshot.MaxReadWaitMS; got != 0 {
		t.Fatalf("MaxReadWaitMS = %d, want 0", got)
	}

	if got := snapshot.ReadWaitMSBuckets["le_1000"]; got != 0 {
		t.Fatalf("read wait le_1000 bucket = %d, want 0", got)
	}

	deliverySnapshot := SnapshotStreamDelivery()
	if got, want := len(deliverySnapshot.Streams), 1; got != want {
		t.Fatalf("len(delivery streams) = %d, want %d", got, want)
	}

	if got := deliverySnapshot.Streams[0].ReadWaitsTotal; got != 0 {
		t.Fatalf("stream ReadWaitsTotal = %d, want 0", got)
	}
}

func TestStartStreamInstrumentationSkipsDeliveryDiagnosticsWhenDebugDisabled(t *testing.T) {
	resetStreamHealthForTest()
	resetStreamDeliveryForTest()
	resetStreamFairnessForTest()
	defer resetStreamFairnessForTest()

	req := httptest.NewRequest(http.MethodGet, "/stream/movie.mkv?link=secret-hash&play", nil)
	req.RemoteAddr = "192.168.1.133:12345"
	rec := httptest.NewRecorder()

	stateCalls := 0
	instrumentation := startStreamInstrumentation(req, rec, false, 77, streamDeliveryMetadata{
		initialOffset:  4096,
		fileSize:       8192,
		requestedRange: true,
		startupState: func() streamStartupCacheSnapshot {
			stateCalls++

			return streamStartupCacheSnapshot{}
		},
	})

	if instrumentation.writer.delivery != nil {
		t.Fatal("debug-disabled instrumentation must not register active stream delivery")
	}

	if _, err := instrumentation.writer.Write([]byte("stream-data")); err != nil {
		t.Fatalf("Write() err = %v", err)
	}

	finishStreamInstrumentation(instrumentation, req.RemoteAddr)
	instrumentation.release()

	if got := SnapshotStreamDelivery().ActiveStreams; got != 0 {
		t.Fatalf("delivery ActiveStreams = %d, want 0", got)
	}

	if got := SnapshotStreamDelivery().BytesWrittenTotal; got != 0 {
		t.Fatalf("delivery BytesWrittenTotal = %d, want 0", got)
	}

	if got := SnapshotStreamHealth().RequestsTotal; got != 0 {
		t.Fatalf("stream health RequestsTotal = %d, want 0", got)
	}

	if stateCalls != 0 {
		t.Fatalf("startup state provider calls = %d, want 0", stateCalls)
	}
}

func TestStartStreamInstrumentationDeliverySnapshotIsPrivacySafe(t *testing.T) {
	resetStreamHealthForTest()
	resetStreamDeliveryForTest()
	resetStreamFairnessForTest()
	defer resetStreamFairnessForTest()

	req := httptest.NewRequest(http.MethodGet, "/stream/private-title.mkv?link=secret-hash&play", nil)
	req.RemoteAddr = "192.168.1.133:12345"
	rec := httptest.NewRecorder()

	instrumentation := startStreamInstrumentation(req, rec, true, 77, streamDeliveryMetadata{
		initialOffset:  4096,
		fileSize:       8192,
		requestedRange: true,
	})
	defer instrumentation.release()

	if instrumentation.writer.delivery == nil {
		t.Fatal("debug-enabled instrumentation must register active stream delivery")
	}

	if _, err := instrumentation.writer.Write([]byte("stream-data")); err != nil {
		t.Fatalf("Write() err = %v", err)
	}

	data, err := json.Marshal(SnapshotStreamDelivery())
	if err != nil {
		t.Fatalf("marshal delivery snapshot: %v", err)
	}

	payload := strings.ToLower(string(data))
	for _, forbidden := range []string{"secret-hash", "private-title", "192.168.1.133", "link=", "query"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("delivery snapshot leaks %q: %s", forbidden, payload)
		}
	}
}

func TestStreamDeliveryRegistryTracksAndRemovesActiveStream(t *testing.T) {
	resetStreamDeliveryForTest()

	started := time.Now().Add(-time.Second)
	delivery, release := registerStreamDeliveryWithMetadata(started, 42, streamDeliveryMetadata{
		initialOffset:  4096,
		fileSize:       8192,
		requestedRange: true,
	})
	delivery.recordWrite(1024, 10*time.Millisecond)

	snapshot := SnapshotStreamDelivery()
	if got, want := snapshot.ActiveStreams, 1; got != want {
		t.Fatalf("ActiveStreams = %d, want %d", got, want)
	}

	if got, want := len(snapshot.Streams), 1; got != want {
		t.Fatalf("len(Streams) = %d, want %d", got, want)
	}

	stream := snapshot.Streams[0]
	if got, want := stream.TorrentID, uint64(42); got != want {
		t.Fatalf("TorrentID = %d, want %d", got, want)
	}

	if stream.ID == 0 {
		t.Fatal("stream ID must be non-zero")
	}

	if got, want := stream.BytesWritten, int64(1024); got != want {
		t.Fatalf("BytesWritten = %d, want %d", got, want)
	}

	if !stream.RequestedRange {
		t.Fatal("RequestedRange = false, want true")
	}

	if got, want := stream.InitialOffset, int64(4096); got != want {
		t.Fatalf("InitialOffset = %d, want %d", got, want)
	}

	if got, want := stream.CurrentOffset, int64(5120); got != want {
		t.Fatalf("CurrentOffset = %d, want %d", got, want)
	}

	if got, want := stream.FileSize, int64(8192); got != want {
		t.Fatalf("FileSize = %d, want %d", got, want)
	}

	if got, want := stream.RemainingBytes, int64(3072); got != want {
		t.Fatalf("RemainingBytes = %d, want %d", got, want)
	}

	if stream.FirstByteMS < 0 {
		t.Fatalf("FirstByteMS = %d, want non-negative", stream.FirstByteMS)
	}

	release()

	afterRelease := SnapshotStreamDelivery()
	if got := afterRelease.ActiveStreams; got != 0 {
		t.Fatalf("ActiveStreams after release = %d, want 0", got)
	}

	if got, want := afterRelease.BytesWrittenTotal, int64(1024); got != want {
		t.Fatalf("BytesWrittenTotal after release = %d, want %d", got, want)
	}
}

func TestStreamDeliverySnapshotClampsCurrentOffsetToFileSize(t *testing.T) {
	resetStreamDeliveryForTest()

	delivery, release := registerStreamDeliveryWithMetadata(time.Now(), 42, streamDeliveryMetadata{
		initialOffset:  7000,
		fileSize:       8192,
		requestedRange: true,
	})
	defer release()

	delivery.recordWrite(4096, time.Millisecond)

	snapshot := SnapshotStreamDelivery()
	if got, want := len(snapshot.Streams), 1; got != want {
		t.Fatalf("len(Streams) = %d, want %d", got, want)
	}

	stream := snapshot.Streams[0]
	if got, want := stream.CurrentOffset, int64(8192); got != want {
		t.Fatalf("CurrentOffset = %d, want %d", got, want)
	}

	if got := stream.RemainingBytes; got != 0 {
		t.Fatalf("RemainingBytes = %d, want 0", got)
	}
}

func TestStreamMetricsWriterRecordsSlowDeliveryWrite(t *testing.T) {
	resetStreamDeliveryForTest()

	delivery, release := registerStreamDelivery(time.Now(), 0)
	defer release()

	rec := &slowResponseWriter{delay: streamSlowWriteThreshold + 20*time.Millisecond}
	writer := &streamMetricsWriter{
		ResponseWriter: rec,
		delivery:       delivery,
	}

	if _, err := writer.Write([]byte("slow-write")); err != nil {
		t.Fatalf("Write() err = %v", err)
	}

	snapshot := SnapshotStreamDelivery()
	if got, want := snapshot.SlowWritesTotal, int64(1); got != want {
		t.Fatalf("SlowWritesTotal = %d, want %d", got, want)
	}

	if snapshot.MaxWriteMS < streamSlowWriteThreshold.Milliseconds() {
		t.Fatalf("MaxWriteMS = %d, want >= %d", snapshot.MaxWriteMS, streamSlowWriteThreshold.Milliseconds())
	}
}

func TestStreamDeliveryRecordsLongWriteThresholds(t *testing.T) {
	resetStreamDeliveryForTest()

	delivery, release := registerStreamDeliveryWithMetadata(time.Now(), 0, streamDeliveryMetadata{
		initialOffset: 4096,
		fileSize:      16 << 20,
	})
	defer release()

	delivery.recordWrite(1, 4*time.Second)
	delivery.recordWrite(1, 11*time.Second)

	snapshot := SnapshotStreamDelivery()
	if got, want := snapshot.SlowWritesTotal, int64(2); got != want {
		t.Fatalf("SlowWritesTotal = %d, want %d", got, want)
	}

	if got, want := snapshot.WritesOver3sTotal, int64(2); got != want {
		t.Fatalf("WritesOver3sTotal = %d, want %d", got, want)
	}

	if got, want := snapshot.WritesOver10sTotal, int64(1); got != want {
		t.Fatalf("WritesOver10sTotal = %d, want %d", got, want)
	}

	if got, want := snapshot.MaxWriteMS, int64(11000); got != want {
		t.Fatalf("MaxWriteMS = %d, want %d", got, want)
	}

	if got, want := len(snapshot.Streams), 1; got != want {
		t.Fatalf("len(Streams) = %d, want %d", got, want)
	}

	stream := snapshot.Streams[0]
	if got, want := stream.LastSlowWriteMS, int64(11000); got != want {
		t.Fatalf("LastSlowWriteMS = %d, want %d", got, want)
	}

	if got, want := stream.LastSlowWriteOffset, int64(4097); got != want {
		t.Fatalf("LastSlowWriteOffset = %d, want %d", got, want)
	}

	if got, want := stream.LastSlowWriteSize, int64(1); got != want {
		t.Fatalf("LastSlowWriteSize = %d, want %d", got, want)
	}

	if got := stream.LastSlowWriteAgeMS; got < 0 {
		t.Fatalf("LastSlowWriteAgeMS = %d, want non-negative", got)
	}
}

func TestStreamDeliveryRecordsWriteGaps(t *testing.T) {
	resetStreamDeliveryForTest()

	delivery, release := registerStreamDelivery(time.Now(), 0)
	defer release()

	delivery.recordWrite(1, time.Millisecond)
	delivery.lastWriteUnixNano.Store(time.Now().Add(-1200 * time.Millisecond).UnixNano())
	delivery.recordWrite(1, time.Millisecond)

	snapshot := SnapshotStreamDelivery()
	if got, want := len(snapshot.Streams), 1; got != want {
		t.Fatalf("len(Streams) = %d, want %d", got, want)
	}

	stream := snapshot.Streams[0]
	if stream.LastWriteGapMS < 1200 {
		t.Fatalf("LastWriteGapMS = %d, want >= 1200", stream.LastWriteGapMS)
	}

	if stream.MaxWriteGapMS < 1200 {
		t.Fatalf("MaxWriteGapMS = %d, want >= 1200", stream.MaxWriteGapMS)
	}

	if got, want := stream.WriteGapsOver250MS, int64(1); got != want {
		t.Fatalf("WriteGapsOver250MS = %d, want %d", got, want)
	}

	if got, want := stream.WriteGapsOver500MS, int64(1); got != want {
		t.Fatalf("WriteGapsOver500MS = %d, want %d", got, want)
	}

	if got, want := stream.WriteGapsOver1000MS, int64(1); got != want {
		t.Fatalf("WriteGapsOver1000MS = %d, want %d", got, want)
	}
}

func TestStreamDeliveryRecordsRollingThroughput(t *testing.T) {
	resetStreamDeliveryForTest()

	delivery, release := registerStreamDelivery(time.Now(), 0)
	defer release()

	delivery.window.startUnixNano = time.Now().Add(-streamDeliveryWindow).UnixNano()
	delivery.window.bytes = 10 << 20
	delivery.recordWrite(10<<20, time.Millisecond)

	snapshot := SnapshotStreamDelivery()
	if got, want := len(snapshot.Streams), 1; got != want {
		t.Fatalf("len(Streams) = %d, want %d", got, want)
	}

	stream := snapshot.Streams[0]
	if stream.Last5sBytesPerSec < 3<<20 {
		t.Fatalf("Last5sBytesPerSec = %d, want at least 3MiB/s", stream.Last5sBytesPerSec)
	}

	if got := stream.Min5sBytesPerSec; got != stream.Last5sBytesPerSec {
		t.Fatalf("Min5sBytesPerSec = %d, want %d", got, stream.Last5sBytesPerSec)
	}
}

func TestStreamDeliveryHeadroomIsUnknownWhenDurationUnavailable(t *testing.T) {
	resetStreamDeliveryForTest()

	delivery, release := registerStreamDeliveryWithMetadata(time.Now(), 0, streamDeliveryMetadata{
		fileSize: 100 << 20,
	})
	defer release()

	delivery.recordWrite(10<<20, time.Millisecond)

	stream := SnapshotStreamDelivery().Streams[0]
	if got := stream.HeadroomStatus; got != "unknown_duration" {
		t.Fatalf("HeadroomStatus = %q, want unknown_duration", got)
	}

	if stream.MediaDurationMS != nil {
		t.Fatalf("MediaDurationMS = %v, want nil", *stream.MediaDurationMS)
	}

	if stream.RequiredBytesPerSec != nil {
		t.Fatalf("RequiredBytesPerSec = %v, want nil", *stream.RequiredBytesPerSec)
	}

	if stream.HeadroomRatio != nil {
		t.Fatalf("HeadroomRatio = %v, want nil", *stream.HeadroomRatio)
	}
}

func TestStreamDeliveryHeadroomForKnownDuration(t *testing.T) {
	resetStreamDeliveryForTest()

	started := time.Now().Add(-2 * time.Second)
	delivery, release := registerStreamDeliveryWithMetadata(started, 0, streamDeliveryMetadata{
		fileSize:      100 << 20,
		mediaDuration: 10 * time.Second,
	})
	defer release()

	delivery.recordWrite(25<<20, time.Millisecond)

	stream := SnapshotStreamDelivery().Streams[0]
	if got := stream.HeadroomStatus; got != "pass" {
		t.Fatalf("HeadroomStatus = %q, want pass", got)
	}

	if stream.MediaDurationMS == nil || *stream.MediaDurationMS != int64(10*time.Second/time.Millisecond) {
		t.Fatalf("MediaDurationMS = %v, want 10000", stream.MediaDurationMS)
	}

	if stream.RequiredBytesPerSec == nil || *stream.RequiredBytesPerSec != 10<<20 {
		t.Fatalf("RequiredBytesPerSec = %v, want %d", stream.RequiredBytesPerSec, 10<<20)
	}

	if stream.HeadroomRatio == nil || *stream.HeadroomRatio < 1.2 {
		t.Fatalf("HeadroomRatio = %v, want >= 1.2", stream.HeadroomRatio)
	}
}

func TestStreamMetricsWriterSkipsDeliveryMetricsWhenDebugDisabled(t *testing.T) {
	resetStreamDeliveryForTest()

	writer := &streamMetricsWriter{ResponseWriter: httptest.NewRecorder()}
	if _, err := writer.Write([]byte("production-fast-path")); err != nil {
		t.Fatalf("Write() err = %v", err)
	}

	snapshot := SnapshotStreamDelivery()
	if got := snapshot.ActiveStreams; got != 0 {
		t.Fatalf("ActiveStreams = %d, want 0", got)
	}

	if got := snapshot.WriteCallsTotal; got != 0 {
		t.Fatalf("WriteCallsTotal = %d, want 0", got)
	}
}

func TestStreamDeliverySnapshotIsPrivacySafe(t *testing.T) {
	resetStreamDeliveryForTest()

	delivery, release := registerStreamDelivery(time.Now(), 7)
	defer release()
	delivery.recordWrite(512, time.Millisecond)

	raw, err := json.Marshal(SnapshotStreamDelivery())
	if err != nil {
		t.Fatalf("Marshal() err = %v", err)
	}

	joined := strings.ToLower(string(raw))
	for _, forbidden := range []string{"hash", "magnet", "path", "title", "ip", "remote"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("snapshot contains privacy-sensitive field %q: %s", forbidden, joined)
		}
	}
}

func TestTorrentRuntimeDiagnosticIDIsStableAndAnonymous(t *testing.T) {
	resetTorrentRuntimeIDsForTest()

	firstHash := metainfo.NewHashFromHex("abcdef1234567890abcdef1234567890abcdef12")
	secondHash := metainfo.NewHashFromHex("1234567890abcdef1234567890abcdef12345678")
	first := &Torrent{TorrentSpec: &torrent.TorrentSpec{AddTorrentOpts: torrent.AddTorrentOpts{InfoHash: firstHash}}}
	same := &Torrent{TorrentSpec: &torrent.TorrentSpec{AddTorrentOpts: torrent.AddTorrentOpts{InfoHash: firstHash}}}
	second := &Torrent{TorrentSpec: &torrent.TorrentSpec{AddTorrentOpts: torrent.AddTorrentOpts{InfoHash: secondHash}}}

	firstID := first.RuntimeDiagnosticID()
	if firstID == 0 {
		t.Fatal("RuntimeDiagnosticID() = 0, want non-zero")
	}

	if got := same.RuntimeDiagnosticID(); got != firstID {
		t.Fatalf("same hash RuntimeDiagnosticID() = %d, want %d", got, firstID)
	}

	if got := second.RuntimeDiagnosticID(); got == 0 || got == firstID {
		t.Fatalf("second hash RuntimeDiagnosticID() = %d, want non-zero id different from %d", got, firstID)
	}
}

func TestStreamFairnessSkipsSingleStream(t *testing.T) {
	resetStreamFairnessForTest()
	defer resetStreamFairnessForTest()

	delays := make([]time.Duration, 0)

	streamFairness.mu.Lock()
	streamFairness.sleep = func(delay time.Duration) {
		delays = append(delays, delay)
	}
	streamFairness.mu.Unlock()

	flow, release := registerStreamFairness(time.Now().Add(-(streamFairnessMinAge + time.Second)), 0)
	defer release()

	flow.recordWriteAndMaybeDelay(int(streamFairnessMinBytes * 4))

	if len(delays) != 0 {
		t.Fatalf("single stream delays = %v, want none", delays)
	}

	if got := SnapshotStreamFairness().DelayedWritesTotal; got != 0 {
		t.Fatalf("DelayedWritesTotal = %d, want 0", got)
	}
}

func TestStreamFairnessSkipsBalancedStreams(t *testing.T) {
	resetStreamFairnessForTest()
	defer resetStreamFairnessForTest()

	delays := make([]time.Duration, 0)

	streamFairness.mu.Lock()
	streamFairness.sleep = func(delay time.Duration) {
		delays = append(delays, delay)
	}
	streamFairness.mu.Unlock()

	started := time.Now().Add(-streamFairnessMinAge - time.Second)
	left, releaseLeft := registerStreamFairness(started, 11)
	right, releaseRight := registerStreamFairness(started, 12)

	defer releaseLeft()
	defer releaseRight()

	left.recordWriteAndMaybeDelay(int(streamFairnessMinBytes * 4))
	right.recordWriteAndMaybeDelay(int(streamFairnessMinBytes * 4))

	if len(delays) != 0 {
		t.Fatalf("balanced stream delays = %v, want none", delays)
	}
}

func TestStreamFairnessSkipsSameTorrentReaders(t *testing.T) {
	resetStreamFairnessForTest()
	defer resetStreamFairnessForTest()

	delays := make([]time.Duration, 0)

	streamFairness.mu.Lock()
	streamFairness.sleep = func(delay time.Duration) {
		delays = append(delays, delay)
	}
	streamFairness.mu.Unlock()

	started := time.Now().Add(-streamFairnessMinAge - time.Second)
	slow, releaseSlow := registerStreamFairness(started, 11)
	fast, releaseFast := registerStreamFairness(started, 11)

	defer releaseSlow()
	defer releaseFast()

	slow.recordWriteAndMaybeDelay(int(streamFairnessMinBytes + 1))
	fast.recordWriteAndMaybeDelay(int(streamFairnessMinBytes * 6))

	if len(delays) != 0 {
		t.Fatalf("same torrent delays = %v, want none", delays)
	}
}

func TestStreamFairnessProtectsSecondUniqueStartup(t *testing.T) {
	resetStreamFairnessForTest()
	resetStreamDeliveryForTest()
	defer resetStreamFairnessForTest()
	defer resetStreamDeliveryForTest()

	delays := make([]time.Duration, 0)

	streamFairness.mu.Lock()
	streamFairness.sleep = func(delay time.Duration) {
		delays = append(delays, delay)
	}
	streamFairness.mu.Unlock()

	matureStarted := time.Now().Add(-streamFairnessMinAge - time.Second)
	startupStarted := time.Now().Add(-2 * time.Second)
	fast, releaseFast := registerStreamFairness(matureStarted, 11)
	startup, releaseStartup := registerStreamFairness(startupStarted, 22)
	startupDelivery, releaseDelivery := registerStreamDelivery(startupStarted, 22)
	startup.setDelivery(startupDelivery)
	fast.bytesWritten.Store(streamFairnessMinBytes * 4)

	defer releaseFast()
	defer releaseStartup()
	defer releaseDelivery()

	snapshotBefore := SnapshotStreamFairness()
	if got, want := snapshotBefore.StartupProtectionActive, 1; got != want {
		t.Fatalf("StartupProtectionActive = %d, want %d", got, want)
	}

	fast.recordWriteAndMaybeDelay(int(streamFairnessMinBytes * 4))

	if len(delays) != 1 {
		t.Fatalf("startup protection delays = %v, want one delay", delays)
	}

	if delays[0] != streamStartupProtectionDelay {
		t.Fatalf("startup protection delay = %s, want %s", delays[0], streamStartupProtectionDelay)
	}

	snapshot := SnapshotStreamFairness()
	if got, want := snapshot.StartupProtectedWrites, int64(1); got != want {
		t.Fatalf("StartupProtectedWrites = %d, want %d", got, want)
	}

	if got, want := snapshot.StartupProtectionDelay, streamStartupProtectionDelay.Microseconds(); got != want {
		t.Fatalf("StartupProtectionDelay = %d, want %d", got, want)
	}
}

func TestStreamFairnessStartupProtectionExitsAfterFirstByte(t *testing.T) {
	resetStreamFairnessForTest()
	resetStreamDeliveryForTest()
	defer resetStreamFairnessForTest()
	defer resetStreamDeliveryForTest()

	delays := make([]time.Duration, 0)

	streamFairness.mu.Lock()
	streamFairness.sleep = func(delay time.Duration) {
		delays = append(delays, delay)
	}
	streamFairness.mu.Unlock()

	matureStarted := time.Now().Add(-streamFairnessMinAge - time.Second)
	startupStarted := time.Now().Add(-2 * time.Second)
	fast, releaseFast := registerStreamFairness(matureStarted, 11)
	startup, releaseStartup := registerStreamFairness(startupStarted, 22)
	startupDelivery, releaseDelivery := registerStreamDelivery(startupStarted, 22)
	startup.setDelivery(startupDelivery)
	startupDelivery.recordWrite(1, time.Millisecond)

	defer releaseFast()
	defer releaseStartup()
	defer releaseDelivery()

	fast.recordWriteAndMaybeDelay(int(streamFairnessMinBytes * 4))

	if len(delays) != 0 {
		t.Fatalf("startup protection after first byte delays = %v, want none", delays)
	}
}

func TestStreamFairnessStartupProtectionSkipsSameTorrentStartup(t *testing.T) {
	resetStreamFairnessForTest()
	resetStreamDeliveryForTest()
	defer resetStreamFairnessForTest()
	defer resetStreamDeliveryForTest()

	delays := make([]time.Duration, 0)

	streamFairness.mu.Lock()
	streamFairness.sleep = func(delay time.Duration) {
		delays = append(delays, delay)
	}
	streamFairness.mu.Unlock()

	matureStarted := time.Now().Add(-streamFairnessMinAge - time.Second)
	startupStarted := time.Now().Add(-2 * time.Second)
	fast, releaseFast := registerStreamFairness(matureStarted, 11)
	startup, releaseStartup := registerStreamFairness(startupStarted, 11)
	startupDelivery, releaseDelivery := registerStreamDelivery(startupStarted, 11)
	startup.setDelivery(startupDelivery)

	defer releaseFast()
	defer releaseStartup()
	defer releaseDelivery()

	fast.recordWriteAndMaybeDelay(int(streamFairnessMinBytes * 4))

	if len(delays) != 0 {
		t.Fatalf("same-torrent startup delays = %v, want none", delays)
	}
}

func TestStreamFairnessStartupProtectionExpiresAndCleansUp(t *testing.T) {
	resetStreamFairnessForTest()
	resetStreamDeliveryForTest()
	defer resetStreamFairnessForTest()
	defer resetStreamDeliveryForTest()

	delays := make([]time.Duration, 0)

	streamFairness.mu.Lock()
	streamFairness.sleep = func(delay time.Duration) {
		delays = append(delays, delay)
	}
	streamFairness.mu.Unlock()

	matureStarted := time.Now().Add(-streamFairnessMinAge - time.Second)
	expiredStarted := time.Now().Add(-streamStartupProtectionWindow - time.Second)
	fast, releaseFast := registerStreamFairness(matureStarted, 11)
	startup, releaseStartup := registerStreamFairness(expiredStarted, 22)
	startupDelivery, releaseDelivery := registerStreamDelivery(expiredStarted, 22)
	startup.setDelivery(startupDelivery)

	defer releaseFast()
	defer releaseDelivery()

	fast.recordWriteAndMaybeDelay(int(streamFairnessMinBytes * 4))

	if len(delays) != 0 {
		t.Fatalf("expired startup protection delays = %v, want none", delays)
	}

	releaseStartup()
	fast.recordWriteAndMaybeDelay(int(streamFairnessMinBytes * 4))

	if got := SnapshotStreamFairness().StartupProtectionActive; got != 0 {
		t.Fatalf("StartupProtectionActive after release = %d, want 0", got)
	}
}

func TestStreamFairnessStartupProtectionWorksWithoutDeliveryDiagnostics(t *testing.T) {
	resetStreamFairnessForTest()
	defer resetStreamFairnessForTest()

	delays := make([]time.Duration, 0)

	streamFairness.mu.Lock()
	streamFairness.sleep = func(delay time.Duration) {
		delays = append(delays, delay)
	}
	streamFairness.mu.Unlock()

	matureStarted := time.Now().Add(-streamFairnessMinAge - time.Second)
	startupStarted := time.Now().Add(-2 * time.Second)
	fast, releaseFast := registerStreamFairness(matureStarted, 11)
	_, releaseStartup := registerStreamFairness(startupStarted, 22)

	defer releaseFast()
	defer releaseStartup()

	fast.recordWriteAndMaybeDelay(int(streamFairnessMinBytes * 4))

	if len(delays) != 1 {
		t.Fatalf("startup protection without diagnostics delays = %v, want one delay", delays)
	}

	if delays[0] != streamStartupProtectionDelay {
		t.Fatalf("startup protection without diagnostics delay = %s, want %s", delays[0], streamStartupProtectionDelay)
	}
}

func TestStreamFairnessAggregatesSameTorrentRangeChurn(t *testing.T) {
	resetStreamFairnessForTest()
	defer resetStreamFairnessForTest()

	delays := make([]time.Duration, 0)

	streamFairness.mu.Lock()
	streamFairness.sleep = func(delay time.Duration) {
		delays = append(delays, delay)
	}
	streamFairness.mu.Unlock()

	started := time.Now().Add(-streamFairnessMinAge - time.Second)
	weak, releaseWeak := registerStreamFairness(started, 11)
	fastProbeA, releaseFastProbeA := registerStreamFairness(started, 12)
	fastProbeB, releaseFastProbeB := registerStreamFairness(started, 12)

	defer releaseWeak()
	defer releaseFastProbeA()
	defer releaseFastProbeB()

	weak.recordWriteAndMaybeDelay(int(streamFairnessMinBytes + 1))
	fastProbeA.recordWriteAndMaybeDelay(int(14 << 20))
	fastProbeB.recordWriteAndMaybeDelay(int(14 << 20))

	if len(delays) != 1 {
		t.Fatalf("same-torrent range churn delays = %v, want one delay", delays)
	}

	if delays[0] != streamFairnessBaseDelay {
		t.Fatalf("same-torrent range churn delay = %s, want %s", delays[0], streamFairnessBaseDelay)
	}

	snapshot := SnapshotStreamFairness()
	if got, want := snapshot.ActiveStreams, 3; got != want {
		t.Fatalf("ActiveStreams = %d, want %d", got, want)
	}

	if got, want := snapshot.ActiveUniqueTorrents, 2; got != want {
		t.Fatalf("ActiveUniqueTorrents = %d, want %d", got, want)
	}

	if got, want := snapshot.ExtraSameTorrentStreams, 1; got != want {
		t.Fatalf("ExtraSameTorrentStreams = %d, want %d", got, want)
	}
}

func TestStreamFairnessDelaysDominantStream(t *testing.T) {
	resetStreamFairnessForTest()
	defer resetStreamFairnessForTest()

	delays := make([]time.Duration, 0)

	streamFairness.mu.Lock()
	streamFairness.sleep = func(delay time.Duration) {
		delays = append(delays, delay)
	}
	streamFairness.mu.Unlock()

	started := time.Now().Add(-streamFairnessMinAge - time.Second)
	slow, releaseSlow := registerStreamFairness(started, 11)
	fast, releaseFast := registerStreamFairness(started, 12)

	defer releaseSlow()
	defer releaseFast()

	slow.recordWriteAndMaybeDelay(int(streamFairnessMinBytes + 1))
	fast.recordWriteAndMaybeDelay(int(streamFairnessMinBytes * 6))

	if len(delays) != 1 {
		t.Fatalf("dominant stream delays = %v, want one delay", delays)
	}

	if delays[0] != streamFairnessMaxDelay {
		t.Fatalf("dominant stream delay = %s, want %s", delays[0], streamFairnessMaxDelay)
	}

	snapshot := SnapshotStreamFairness()
	if got, want := snapshot.ActiveStreams, 2; got != want {
		t.Fatalf("ActiveStreams = %d, want %d", got, want)
	}

	seenTorrentIDs := map[uint64]bool{}
	for _, stream := range snapshot.Streams {
		seenTorrentIDs[stream.TorrentID] = true
	}

	if !seenTorrentIDs[11] || !seenTorrentIDs[12] {
		t.Fatalf("fairness torrent ids = %v, want 11 and 12", seenTorrentIDs)
	}

	if got, want := snapshot.DelayedWritesTotal, int64(1); got != want {
		t.Fatalf("DelayedWritesTotal = %d, want %d", got, want)
	}

	if got, want := snapshot.MaxDelayMicros, streamFairnessMaxDelay.Microseconds(); got != want {
		t.Fatalf("MaxDelayMicros = %d, want %d", got, want)
	}
}

func TestStreamFairnessDelaysDominantStreamWhenWeakStreamHasReadWaits(t *testing.T) {
	resetStreamFairnessForTest()
	resetStreamDeliveryForTest()
	defer resetStreamFairnessForTest()
	defer resetStreamDeliveryForTest()

	delays := make([]time.Duration, 0)

	streamFairness.mu.Lock()
	streamFairness.sleep = func(delay time.Duration) {
		delays = append(delays, delay)
	}
	streamFairness.mu.Unlock()

	started := time.Now().Add(-streamFairnessMinAge - time.Second)
	weak, releaseWeak := registerStreamFairness(started, 11)
	fast, releaseFast := registerStreamFairness(started, 12)
	weakDelivery, releaseDelivery := registerStreamDelivery(started, 11)
	weak.setDelivery(weakDelivery)
	weakDelivery.recordReadWait(streamReadWaitThreshold+time.Millisecond, 0, 32768)

	defer releaseWeak()
	defer releaseFast()
	defer releaseDelivery()

	weak.recordWriteAndMaybeDelay(int(streamFairnessMinBytes + 1))
	fast.recordWriteAndMaybeDelay(int(streamFairnessMinBytes * 6))

	if len(delays) != 1 {
		t.Fatalf("read-wait limited weak stream delays = %v, want one delay", delays)
	}

	if delays[0] != streamFairnessMaxDelay {
		t.Fatalf("read-wait limited weak stream delay = %s, want %s", delays[0], streamFairnessMaxDelay)
	}
}

func TestStreamFairnessDelaysDominantStreamWithMinorReadWaits(t *testing.T) {
	resetStreamFairnessForTest()
	resetStreamDeliveryForTest()
	defer resetStreamFairnessForTest()
	defer resetStreamDeliveryForTest()

	delays := make([]time.Duration, 0)

	streamFairness.mu.Lock()
	streamFairness.sleep = func(delay time.Duration) {
		delays = append(delays, delay)
	}
	streamFairness.mu.Unlock()

	started := time.Now().Add(-streamFairnessMinAge - time.Second)
	weak, releaseWeak := registerStreamFairness(started, 11)
	fast, releaseFast := registerStreamFairness(started, 12)
	fastDelivery, releaseDelivery := registerStreamDelivery(started, 12)
	fast.setDelivery(fastDelivery)
	fastDelivery.recordReadWait(streamReadWaitThreshold+time.Millisecond, 0, 32768)

	defer releaseWeak()
	defer releaseFast()
	defer releaseDelivery()

	weak.recordWriteAndMaybeDelay(int(streamFairnessMinBytes + 1))
	fast.recordWriteAndMaybeDelay(int(streamFairnessMinBytes * 6))

	if len(delays) != 1 {
		t.Fatalf("minor-read-wait dominant stream delays = %v, want one delay", delays)
	}
}

func TestStreamFairnessSkipsWhenDominantStreamHasRecentSevereReadWait(t *testing.T) {
	resetStreamFairnessForTest()
	resetStreamDeliveryForTest()
	defer resetStreamFairnessForTest()
	defer resetStreamDeliveryForTest()

	delays := make([]time.Duration, 0)

	streamFairness.mu.Lock()
	streamFairness.sleep = func(delay time.Duration) {
		delays = append(delays, delay)
	}
	streamFairness.mu.Unlock()

	started := time.Now().Add(-streamFairnessMinAge - time.Second)
	weak, releaseWeak := registerStreamFairness(started, 11)
	fast, releaseFast := registerStreamFairness(started, 12)
	fastDelivery, releaseDelivery := registerStreamDelivery(started, 12)
	fast.setDelivery(fastDelivery)
	fastDelivery.recordReadWait(4*time.Second, 0, 32768)

	defer releaseWeak()
	defer releaseFast()
	defer releaseDelivery()

	weak.recordWriteAndMaybeDelay(int(streamFairnessMinBytes + 1))
	fast.recordWriteAndMaybeDelay(int(streamFairnessMinBytes * 6))

	if len(delays) != 0 {
		t.Fatalf("recent severe-read-wait dominant stream delays = %v, want none", delays)
	}
}

func TestStreamFairnessSkipsWhenWeakStreamHasSlowWrites(t *testing.T) {
	resetStreamFairnessForTest()
	resetStreamDeliveryForTest()
	defer resetStreamFairnessForTest()
	defer resetStreamDeliveryForTest()

	delays := make([]time.Duration, 0)

	streamFairness.mu.Lock()
	streamFairness.sleep = func(delay time.Duration) {
		delays = append(delays, delay)
	}
	streamFairness.mu.Unlock()

	started := time.Now().Add(-streamFairnessMinAge - time.Second)
	weak, releaseWeak := registerStreamFairness(started, 11)
	fast, releaseFast := registerStreamFairness(started, 12)
	weakDelivery, releaseDelivery := registerStreamDelivery(started, 11)
	weak.setDelivery(weakDelivery)
	weakDelivery.recordWrite(1, streamSlowWriteThreshold+time.Millisecond)

	defer releaseWeak()
	defer releaseFast()
	defer releaseDelivery()

	weak.recordWriteAndMaybeDelay(int(streamFairnessMinBytes + 1))
	fast.recordWriteAndMaybeDelay(int(streamFairnessMinBytes * 6))

	if len(delays) != 0 {
		t.Fatalf("client-backpressure limited weak stream delays = %v, want none", delays)
	}
}

func TestStreamFairnessReleaseRemovesActiveStream(t *testing.T) {
	resetStreamFairnessForTest()
	defer resetStreamFairnessForTest()

	_, release := registerStreamFairness(time.Now(), 0)

	if got, want := SnapshotStreamFairness().ActiveStreams, 1; got != want {
		t.Fatalf("ActiveStreams = %d, want %d", got, want)
	}

	release()

	if got := SnapshotStreamFairness().ActiveStreams; got != 0 {
		t.Fatalf("ActiveStreams after release = %d, want 0", got)
	}
}

func TestStreamHealthClassifiesClientDisconnect(t *testing.T) {
	resetStreamHealthForTest()

	recordStreamCompleted(0, 512, syscall.EPIPE)
	recordStreamCompleted(0, 512, errors.New("write tcp: broken pipe"))

	snapshot := SnapshotStreamHealth()

	if got, want := snapshot.ClientDisconnectsTotal, int64(2); got != want {
		t.Fatalf("ClientDisconnectsTotal = %d, want %d", got, want)
	}
}

func TestRequestedRangeStart(t *testing.T) {
	tests := []struct {
		name       string
		rangeValue string
		size       int64
		want       int64
		ok         bool
	}{
		{name: "simple open range", rangeValue: "bytes=123-", size: 1000, want: 123, ok: true},
		{name: "bounded range", rangeValue: "bytes=5-99", size: 1000, want: 5, ok: true},
		{name: "suffix range unsupported", rangeValue: "bytes=-500", size: 1000, ok: false},
		{name: "multiple ranges unsupported", rangeValue: "bytes=0-1,4-5", size: 1000, ok: false},
		{name: "out of bounds", rangeValue: "bytes=1000-", size: 1000, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
			if err != nil {
				t.Fatalf("NewRequest() error = %v", err)
			}

			req.Header.Set("Range", tt.rangeValue)

			got, ok := requestedRangeStart(req, tt.size)
			if ok != tt.ok {
				t.Fatalf("requestedRangeStart(%q, %d) ok = %v, want %v", tt.rangeValue, tt.size, ok, tt.ok)
			}

			if got != tt.want {
				t.Fatalf("requestedRangeStart(%q, %d) = %d, want %d", tt.rangeValue, tt.size, got, tt.want)
			}
		})
	}
}

func TestInitialStreamOffset(t *testing.T) {
	tests := []struct {
		name       string
		rangeValue string
		size       int64
		want       int64
	}{
		{name: "defaults to zero", size: 1000, want: 0},
		{name: "uses requested range start", rangeValue: "bytes=123-", size: 1000, want: 123},
		{name: "ignores unsupported suffix range", rangeValue: "bytes=-200", size: 1000, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
			if err != nil {
				t.Fatalf("NewRequest() error = %v", err)
			}

			if tt.rangeValue != "" {
				req.Header.Set("Range", tt.rangeValue)
			}

			if got := initialStreamOffset(req, tt.size); got != tt.want {
				t.Fatalf("initialStreamOffset(%q, %d) = %d, want %d", tt.rangeValue, tt.size, got, tt.want)
			}
		})
	}
}
