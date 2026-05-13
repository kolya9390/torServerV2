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

type streamMetricsWriter struct {
	http.ResponseWriter

	firstWriteUnixNano atomic.Int64
	bytesWritten       atomic.Int64
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

	n, err := w.ResponseWriter.Write(p)
	w.bytesWritten.Add(int64(n))

	return n, err
}

func (w *streamMetricsWriter) ReadFrom(r io.Reader) (int64, error) {
	tr := &firstByteTrackingReader{
		reader: r,
		mark:   w.markFirstWrite,
	}
	rf, ok := w.ResponseWriter.(io.ReaderFrom)

	if !ok {
		return io.Copy(writeOnly{Writer: w}, tr)
	}

	n, err := rf.ReadFrom(tr)
	w.bytesWritten.Add(n)

	return n, err
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
	reader io.Reader
	mark   func()
	seen   bool
}

func (r *firstByteTrackingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 && !r.seen {
		r.seen = true
		r.mark()
	}

	return n, err
}

// serveContentReadSeeker shields the underlying torrent reader from
// ServeContent's size probe (SeekEnd/SeekStart).
type serveContentReadSeeker struct {
	reader  streamContentSource
	size    int64
	pos     int64
	virtual bool
}

func newServeContentReadSeeker(reader streamContentSource, size int64) *serveContentReadSeeker {
	pos := int64(0)
	if reader != nil {
		pos = reader.Offset()
	}

	return &serveContentReadSeeker{
		reader: reader,
		size:   size,
		pos:    pos,
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

	n, err := s.reader.Read(p)
	s.pos += int64(n)

	return n, err
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
