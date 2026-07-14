package torr

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

type streamContentSource interface {
	io.ReadSeeker
	Offset() int64
}

const streamCopyBufferSize = 256 << 10

type streamMetricsWriter struct {
	http.ResponseWriter

	firstWriteUnixNano atomic.Int64
	bytesWritten       atomic.Int64
	lastError          atomic.Value
	trackReadWait      bool
	delivery           *streamDelivery
	fairness           *streamFairnessFlow
}

type writeOnly struct {
	io.Writer
}

func (w *streamMetricsWriter) markFirstWrite() {
	now := time.Now().UnixNano()
	_ = w.firstWriteUnixNano.CompareAndSwap(0, now)
}

func (w *streamMetricsWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		w.markFirstWrite()
	}

	var started time.Time
	if w.delivery != nil {
		started = time.Now()
	}

	n, err := w.ResponseWriter.Write(p)
	w.bytesWritten.Add(int64(n))

	if w.trackReadWait {
		recordStreamBytesWritten(int64(n))
	}

	if w.delivery != nil {
		w.delivery.recordWrite(n, time.Since(started))
	}

	if w.fairness != nil {
		w.fairness.recordWriteAndMaybeDelay(n)
	}

	w.recordError(err)

	return n, err
}

func (w *streamMetricsWriter) ReadFrom(r io.Reader) (int64, error) {
	tr := &firstByteTrackingReader{
		reader:        r,
		mark:          w.markFirstWrite,
		trackReadWait: w.trackReadWait,
		delivery:      w.delivery,
	}

	if w.trackReadWait {
		return copyStreamContent(writeOnly{Writer: w}, tr)
	}

	rf, ok := w.ResponseWriter.(io.ReaderFrom)
	if !ok {
		return copyStreamContent(writeOnly{Writer: w}, tr)
	}

	tr.fairness = w.fairness

	n, err := rf.ReadFrom(tr)
	w.bytesWritten.Add(n)
	w.recordError(err)

	return n, err
}

func copyStreamContent(dst io.Writer, src io.Reader) (int64, error) {
	return io.CopyBuffer(dst, src, make([]byte, streamCopyBufferSize))
}

func (w *streamMetricsWriter) recordError(err error) {
	if err != nil {
		w.lastError.Store(err)
	}
}

func (w *streamMetricsWriter) err() error {
	value := w.lastError.Load()
	if value == nil {
		return nil
	}

	err, ok := value.(error)
	if !ok {
		return nil
	}

	return err
}

func (w *streamMetricsWriter) Flush() {
	if fl, ok := w.ResponseWriter.(http.Flusher); ok {
		fl.Flush()
	}
}

func (w *streamMetricsWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}

	return hj.Hijack()
}

func (w *streamMetricsWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}

	return http.ErrNotSupported
}

type firstByteTrackingReader struct {
	reader        io.Reader
	mark          func()
	trackReadWait bool
	delivery      *streamDelivery
	fairness      *streamFairnessFlow
	seen          bool
}

type streamOffsetReader interface {
	Offset() int64
}

func (r *firstByteTrackingReader) Read(p []byte) (int, error) {
	var started time.Time

	offset := int64(-1)

	if r.trackReadWait {
		started = time.Now()
		offset = streamReadOffset(r.reader)
	}

	n, err := r.reader.Read(p)

	if r.trackReadWait {
		wait := time.Since(started)
		recordStreamReadWait(wait)
		r.delivery.recordReadWait(wait, offset, len(p))
	}

	if n > 0 && !r.seen {
		r.seen = true
		r.mark()
	}

	if r.fairness != nil {
		r.fairness.recordWriteAndMaybeDelay(n)
	}

	return n, err
}

func streamReadOffset(reader io.Reader) int64 {
	offsetReader, ok := reader.(streamOffsetReader)
	if !ok {
		return -1
	}

	return offsetReader.Offset()
}

// serveContentReadSeeker shields the underlying torrent reader from
// ServeContent's size probe (SeekEnd/SeekStart).
type serveContentReadSeeker struct {
	reader       streamContentSource
	size         int64
	pos          int64
	virtual      bool
	readObserver *streamDelivery
}

func newServeContentReadSeeker(reader streamContentSource, size int64, readObserver *streamDelivery) *serveContentReadSeeker {
	pos := int64(0)
	if reader != nil {
		pos = reader.Offset()
	}

	return &serveContentReadSeeker{
		reader:       reader,
		size:         size,
		pos:          pos,
		readObserver: readObserver,
	}
}

func (s *serveContentReadSeeker) Read(p []byte) (int, error) {
	if s.reader == nil {
		return 0, io.EOF
	}

	if s.virtual {
		if _, err := s.seekReal(s.pos); err != nil {
			return 0, err
		}
	}

	started := time.Now()
	offset := s.pos
	n, err := s.reader.Read(p)
	s.recordReadWaitLocation(time.Since(started), offset, len(p))
	s.pos += int64(n)

	return n, err
}

func (s *serveContentReadSeeker) recordReadWaitLocation(wait time.Duration, offset int64, requestedBytes int) {
	if s.readObserver == nil {
		return
	}

	s.readObserver.recordReadWaitLocation(wait, offset, requestedBytes)
}

func (s *serveContentReadSeeker) Offset() int64 {
	if s == nil {
		return -1
	}

	return s.pos
}

func (s *serveContentReadSeeker) Seek(offset int64, whence int) (int64, error) {
	target, err := s.resolveOffset(offset, whence)
	if err != nil {
		return 0, err
	}

	if whence == io.SeekEnd && offset == 0 {
		s.pos = target
		s.virtual = true

		return s.pos, nil
	}

	if s.virtual {
		s.pos = target

		return s.pos, nil
	}

	return s.seekReal(target)
}

func (s *serveContentReadSeeker) seekReal(target int64) (int64, error) {
	if s.reader == nil {
		return 0, io.EOF
	}

	pos, err := s.reader.Seek(target, io.SeekStart)
	if err != nil {
		return 0, err
	}

	s.pos = pos
	s.virtual = false

	return pos, nil
}

func (s *serveContentReadSeeker) resolveOffset(offset int64, whence int) (int64, error) {
	var base int64

	switch whence {
	case io.SeekStart:
		base = 0
	case io.SeekCurrent:
		base = s.pos
	case io.SeekEnd:
		base = s.size
	default:
		return 0, errors.New("invalid whence")
	}

	target := base + offset
	if target < 0 {
		return 0, errors.New("negative position")
	}

	return target, nil
}
