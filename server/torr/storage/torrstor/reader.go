package torrstor

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anacrolix/torrent"

	"server/log"
)

type Reader struct {
	torrent.Reader

	offset    atomic.Int64
	readahead atomic.Int64
	file      *torrent.File

	cache    *Cache
	isClosed atomic.Bool

	///Preload
	lastAccess atomic.Int64
	isUse      atomic.Bool
	mu         sync.Mutex
}

func newReader(file *torrent.File, cache *Cache) *Reader {
	r := &Reader{
		file:  file,
		cache: cache,
	}
	r.Reader = file.NewReader()
	r.isUse.Store(true)
	cache.readers.active.Add(1)

	cache.readers.mu.Lock()
	cache.readers.items[r] = struct{}{}
	cache.readers.mu.Unlock()

	return r
}

func (r *Reader) Seek(offset int64, whence int) (n int64, err error) {
	r.mu.Lock()
	if r.isClosed.Load() {
		r.mu.Unlock()

		return 0, io.EOF
	}

	r.readerOnLocked()
	n, err = r.Reader.Seek(offset, whence)
	r.offset.Store(n)
	r.lastAccess.Store(time.Now().Unix())
	r.mu.Unlock()

	return
}

func (r *Reader) Read(p []byte) (n int, err error) {
	err = io.EOF

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isClosed.Load() {
		return
	}

	if r.file.Torrent() != nil && r.file.Torrent().Info() != nil {
		r.readerOnLocked()
		n, err = r.Reader.Read(p)

		// samsung tv fix xvid/divx
		//if r.offset == 0 && len(p) >= 192 {
		//	str := strings.ToLower(string(p[112:116]))
		//	if str == "xvid" || str == "divx" {
		//		p[112] = 0x4D // M
		//		p[113] = 0x50 // P
		//		p[114] = 0x34 // 4
		//		p[115] = 0x56 // V
		//	}
		//	str = strings.ToLower(string(p[188:192]))
		//	if str == "xvid" || str == "divx" {
		//		p[188] = 0x4D // M
		//		p[189] = 0x50 // P
		//		p[190] = 0x34 // 4
		//		p[191] = 0x56 // V
		//	}
		//}

		r.offset.Add(int64(n))
		r.lastAccess.Store(time.Now().Unix())
	} else {
		log.TLogln("Torrent closed and readed")
	}

	return
}

func (r *Reader) clampReadahead(length int64) int64 {
	if r.cache != nil && length > r.cache.GetCapacity() {
		length = r.cache.GetCapacity()
	}

	return length
}

func (r *Reader) SetContext(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isClosed.Load() {
		return
	}

	r.Reader.SetContext(ctx)
}

func (r *Reader) SetResponsive() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isClosed.Load() {
		return
	}

	r.Reader.SetResponsive()
}

func (r *Reader) SetReadahead(length int64) {
	length = r.clampReadahead(length)

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.readahead.Load() == length {
		return
	}

	r.readahead.Store(length)

	if r.isUse.Load() {
		r.Reader.SetReadahead(length)
	}
}

func (r *Reader) SetIdle() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isClosed.Load() {
		return
	}

	r.readerOffLocked()
}

func (r *Reader) Offset() int64 {
	return r.offset.Load()
}

func (r *Reader) Readahead() int64 {
	return r.readahead.Load()
}

func (r *Reader) Close() {
	// file reader close in gotorrent
	// this struct close in cache
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isClosed.Swap(true) {
		return
	}

	if r.isUse.Swap(false) {
		r.cache.readers.active.Add(-1)
	}

	if len(r.file.Torrent().Files()) > 0 {
		_ = r.Reader.Close()
	}

	r.cache.queueCleanPieces()
}

func (r *Reader) getPiecesRange() Range {
	return r.getPiecesRangeForReaders(r.getUseReaders())
}

func (r *Reader) getPiecesRangeForReaders(activeReaders int) Range {
	startOff, endOff := r.getOffsetRangeForReaders(activeReaders)

	return Range{r.getPieceNum(startOff), r.getPieceNum(endOff), r.file}
}

func (r *Reader) getReaderPiece() int {
	return r.getPieceNum(r.offset.Load())
}

func (r *Reader) getReaderRAHPiece() int {
	return r.getPieceNum(r.offset.Load() + r.readahead.Load())
}

func (r *Reader) getPieceNum(offset int64) int {
	return int((offset + r.file.Offset()) / r.cache.pieceLength)
}

func (r *Reader) getOffsetRange() (int64, int64) {
	return r.getOffsetRangeForReaders(r.getUseReaders())
}

func (r *Reader) getOffsetRangeForReaders(activeReaders int) (int64, int64) {
	prc := int64(r.cache.currentPlaybackConfig().ReadAheadPct)
	if prc <= 0 || prc > 100 {
		prc = 95
	}

	offset := r.offset.Load()

	readers := int64(activeReaders)
	if readers == 0 {
		readers = 1
	}

	perReaderWindow := r.cache.capacity / readers
	beginOffset := offset - perReaderWindow*(100-prc)/100
	endOffset := offset + perReaderWindow*prc/100

	if beginOffset < 0 {
		beginOffset = 0
	}

	if r.file != nil && endOffset > r.file.Length() {
		endOffset = r.file.Length()
	}

	return beginOffset, endOffset
}

func (r *Reader) checkReader(totalReaders int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if time.Now().Unix() > r.lastAccess.Load()+60 && totalReaders > 1 {
		r.readerOffLocked()
	} else {
		r.readerOnLocked()
	}
}

func (r *Reader) readerOnLocked() {
	if !r.isUse.Load() {
		if pos, err := r.Reader.Seek(0, io.SeekCurrent); err == nil && pos == 0 {
			if _, err := r.Reader.Seek(r.offset.Load(), io.SeekStart); err != nil {
				log.TLogln("readerOn seek error:", err)
			}
		}

		r.Reader.SetReadahead(r.readahead.Load())
		r.isUse.Store(true)

		if r.cache != nil {
			r.cache.readers.active.Add(1)
		}
	}
}

func (r *Reader) readerOffLocked() {
	if r.isUse.Load() {
		r.Reader.SetReadahead(0)

		r.isUse.Store(false)

		if r.cache != nil {
			r.cache.readers.active.Add(-1)
		}

		if r.offset.Load() > 0 {
			if _, err := r.Reader.Seek(0, io.SeekStart); err != nil {
				log.TLogln("readerOff seek error:", err)
			}
		}
	}
}

func (r *Reader) getUseReaders() int {
	if r.cache == nil {
		return 0
	}

	return r.cache.GetUseReaders()
}

func (r *Reader) isActive() bool {
	return r.isUse.Load()
}
