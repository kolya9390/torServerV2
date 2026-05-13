package torr

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

type fakeStreamContentSource struct {
	data  []byte
	pos   int64
	seeks [][2]int64
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
		wantBytes int64
	}{
		{name: "fixed reader window", pieceLen: 2 << 20, cacheCap: 64 << 20, wantBytes: 16 << 20},
		{name: "fixed oversized cache", pieceLen: 2 << 20, cacheCap: 256 << 20, wantBytes: 16 << 20},
		{name: "falls back to fixed baseline", pieceLen: 4 << 20, cacheCap: 0, wantBytes: 16 << 20},
		{name: "respects tiny cache", pieceLen: 1 << 20, cacheCap: 6 << 20, wantBytes: 6 << 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := streamReaderReadahead(tt.pieceLen, tt.cacheCap); got != tt.wantBytes {
				t.Fatalf("streamReaderReadahead(%d, %d) = %d, want %d", tt.pieceLen, tt.cacheCap, got, tt.wantBytes)
			}
		})
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
	t.Parallel()

	rec := httptest.NewRecorder()
	w := &streamMetricsWriter{ResponseWriter: rec}

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
}

func TestStreamMetricsWriter_ReadFrom(t *testing.T) {
	t.Parallel()

	rec := &readerFromRecorder{}
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

	if got := w.firstWriteUnixNano.Load(); got == 0 {
		t.Fatal("firstWriteUnixNano was not recorded")
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
