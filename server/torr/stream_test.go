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

type fakeContextReader struct {
	ctx context.Context
}

func (r *fakeContextReader) SetContext(ctx context.Context) {
	r.ctx = ctx
}

func (r *slowOnceReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}

	time.Sleep(r.delay)
	r.done = true

	return copy(p, r.data), nil
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
	wrapped := newServeContentReadSeeker(src, int64(len(src.data)))

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
	wrapped := newServeContentReadSeeker(src, int64(len(src.data)))

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

func TestStreamReaderReadahead(t *testing.T) {
	tests := []struct {
		name      string
		pieceLen  int64
		cacheCap  int64
		readers   int
		wantBytes int64
	}{
		{name: "fixed reader window", pieceLen: 2 << 20, cacheCap: 64 << 20, readers: 1, wantBytes: 16 << 20},
		{name: "fixed oversized cache", pieceLen: 2 << 20, cacheCap: 256 << 20, readers: 1, wantBytes: 16 << 20},
		{name: "falls back to fixed baseline", pieceLen: 4 << 20, cacheCap: 0, readers: 1, wantBytes: 16 << 20},
		{name: "respects tiny cache", pieceLen: 1 << 20, cacheCap: 6 << 20, readers: 1, wantBytes: 6 << 20},
		{name: "narrows two reader cache share", pieceLen: 2 << 20, cacheCap: 64 << 20, readers: 2, wantBytes: 8 << 20},
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

func TestStreamDeliveryRegistryTracksAndRemovesActiveStream(t *testing.T) {
	resetStreamDeliveryForTest()

	started := time.Now().Add(-time.Second)
	delivery, release := registerStreamDelivery(started, 42)
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

	delivery, release := registerStreamDelivery(time.Now(), 0)
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
	left, releaseLeft := registerStreamFairness(started, 0)
	right, releaseRight := registerStreamFairness(started, 0)

	defer releaseLeft()
	defer releaseRight()

	left.recordWriteAndMaybeDelay(int(streamFairnessMinBytes * 4))
	right.recordWriteAndMaybeDelay(int(streamFairnessMinBytes * 4))

	if len(delays) != 0 {
		t.Fatalf("balanced stream delays = %v, want none", delays)
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
