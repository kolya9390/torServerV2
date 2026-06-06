package torr

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	streamSlowWriteThreshold      = 500 * time.Millisecond
	streamVerySlowWriteThreshold  = 3 * time.Second
	streamSevereWriteThreshold    = 10 * time.Second
	maxStreamDeliverySnapshotSize = 64
)

var (
	streamDeliveryID atomic.Uint64
	streamDeliveries = streamDeliveryRegistry{
		active: make(map[uint64]*streamDelivery),
	}
)

type streamDeliveryRegistry struct {
	mu sync.RWMutex

	active             map[uint64]*streamDelivery
	bytesWrittenTotal  atomic.Int64
	writeCallsTotal    atomic.Int64
	slowWritesTotal    atomic.Int64
	writesOver3sTotal  atomic.Int64
	writesOver10sTotal atomic.Int64
	maxWriteMS         atomic.Int64
}

type streamDelivery struct {
	id uint64

	torrentID          uint64
	startedUnixNano    int64
	firstWriteUnixNano atomic.Int64
	lastWriteUnixNano  atomic.Int64
	bytesWritten       atomic.Int64
	writeCalls         atomic.Int64
	slowWritesTotal    atomic.Int64
	writesOver3sTotal  atomic.Int64
	writesOver10sTotal atomic.Int64
	maxWriteMS         atomic.Int64
	readWaitsTotal     atomic.Int64
	readWaitTotalMS    atomic.Int64
	readWaitsOver3s    atomic.Int64
	readWaitsOver10s   atomic.Int64
	maxReadWaitMS      atomic.Int64
}

type streamDeliverySnapshot struct {
	ActiveStreams      int                      `json:"active_streams"`
	BytesWrittenTotal  int64                    `json:"bytes_written_total"`
	WriteCallsTotal    int64                    `json:"write_calls_total"`
	SlowWritesTotal    int64                    `json:"slow_writes_total"`
	WritesOver3sTotal  int64                    `json:"writes_over_3s_total"`
	WritesOver10sTotal int64                    `json:"writes_over_10s_total"`
	MaxWriteMS         int64                    `json:"max_write_ms"`
	Streams            []activeDeliverySnapshot `json:"streams"`
}

type activeDeliverySnapshot struct {
	TorrentID          uint64 `json:"torrent_id"`
	ID                 uint64 `json:"id"`
	ElapsedMS          int64  `json:"elapsed_ms"`
	FirstByteMS        int64  `json:"first_byte_ms"`
	SinceLastWriteMS   int64  `json:"since_last_write_ms"`
	BytesWritten       int64  `json:"bytes_written"`
	BytesPerSecond     int64  `json:"bytes_per_second"`
	WriteCalls         int64  `json:"write_calls"`
	SlowWritesTotal    int64  `json:"slow_writes_total"`
	WritesOver3sTotal  int64  `json:"writes_over_3s_total"`
	WritesOver10sTotal int64  `json:"writes_over_10s_total"`
	MaxWriteMS         int64  `json:"max_write_ms"`
	ReadWaitsTotal     int64  `json:"read_waits_total"`
	ReadWaitTotalMS    int64  `json:"read_wait_total_ms"`
	ReadWaitsOver3s    int64  `json:"read_waits_over_3s_total"`
	ReadWaitsOver10s   int64  `json:"read_waits_over_10s_total"`
	MaxReadWaitMS      int64  `json:"max_read_wait_ms"`
}

func registerStreamDelivery(started time.Time, torrentID uint64) (*streamDelivery, func()) {
	delivery := &streamDelivery{
		id:              streamDeliveryID.Add(1),
		torrentID:       torrentID,
		startedUnixNano: started.UnixNano(),
	}

	streamDeliveries.mu.Lock()
	streamDeliveries.active[delivery.id] = delivery
	streamDeliveries.mu.Unlock()

	return delivery, func() {
		streamDeliveries.mu.Lock()
		delete(streamDeliveries.active, delivery.id)
		streamDeliveries.mu.Unlock()
	}
}

func (d *streamDelivery) recordWrite(bytesWritten int, elapsed time.Duration) {
	if d == nil {
		return
	}

	now := time.Now().UnixNano()
	_ = d.firstWriteUnixNano.CompareAndSwap(0, now)
	d.lastWriteUnixNano.Store(now)
	d.writeCalls.Add(1)
	d.bytesWritten.Add(int64(bytesWritten))
	streamDeliveries.writeCallsTotal.Add(1)
	streamDeliveries.bytesWrittenTotal.Add(int64(bytesWritten))

	elapsedMS := elapsed.Milliseconds()
	updateMaxAtomic(&d.maxWriteMS, elapsedMS)
	updateMaxAtomic(&streamDeliveries.maxWriteMS, elapsedMS)

	if elapsed >= streamSlowWriteThreshold {
		d.slowWritesTotal.Add(1)
		streamDeliveries.slowWritesTotal.Add(1)
	}

	if elapsed > streamVerySlowWriteThreshold {
		d.writesOver3sTotal.Add(1)
		streamDeliveries.writesOver3sTotal.Add(1)
	}

	if elapsed > streamSevereWriteThreshold {
		d.writesOver10sTotal.Add(1)
		streamDeliveries.writesOver10sTotal.Add(1)
	}
}

func (d *streamDelivery) recordReadWait(wait time.Duration) {
	if d == nil || wait < streamReadWaitThreshold {
		return
	}

	waitMS := wait.Milliseconds()

	d.readWaitsTotal.Add(1)
	d.readWaitTotalMS.Add(waitMS)
	updateMaxAtomic(&d.maxReadWaitMS, waitMS)

	if wait > 3*time.Second {
		d.readWaitsOver3s.Add(1)
	}

	if wait > 10*time.Second {
		d.readWaitsOver10s.Add(1)
	}
}

func SnapshotStreamDelivery() streamDeliverySnapshot {
	now := time.Now()
	snapshot := streamDeliverySnapshot{
		BytesWrittenTotal:  streamDeliveries.bytesWrittenTotal.Load(),
		WriteCallsTotal:    streamDeliveries.writeCallsTotal.Load(),
		SlowWritesTotal:    streamDeliveries.slowWritesTotal.Load(),
		WritesOver3sTotal:  streamDeliveries.writesOver3sTotal.Load(),
		WritesOver10sTotal: streamDeliveries.writesOver10sTotal.Load(),
		MaxWriteMS:         streamDeliveries.maxWriteMS.Load(),
		Streams:            make([]activeDeliverySnapshot, 0),
	}

	streamDeliveries.mu.RLock()
	defer streamDeliveries.mu.RUnlock()

	snapshot.ActiveStreams = len(streamDeliveries.active)
	snapshot.Streams = make([]activeDeliverySnapshot, 0, min(len(streamDeliveries.active), maxStreamDeliverySnapshotSize))

	for _, delivery := range streamDeliveries.active {
		if len(snapshot.Streams) >= maxStreamDeliverySnapshotSize {
			break
		}

		item := delivery.snapshot(now)
		snapshot.Streams = append(snapshot.Streams, item)
	}

	return snapshot
}

func (d *streamDelivery) snapshot(now time.Time) activeDeliverySnapshot {
	elapsed := now.Sub(time.Unix(0, d.startedUnixNano))
	bytesWritten := d.bytesWritten.Load()

	return activeDeliverySnapshot{
		TorrentID:          d.torrentID,
		ID:                 d.id,
		ElapsedMS:          elapsed.Milliseconds(),
		FirstByteMS:        durationSinceStartMS(d.startedUnixNano, d.firstWriteUnixNano.Load()),
		SinceLastWriteMS:   durationSinceEventMS(now, d.lastWriteUnixNano.Load()),
		BytesWritten:       bytesWritten,
		BytesPerSecond:     bytesPerSecond(bytesWritten, elapsed),
		WriteCalls:         d.writeCalls.Load(),
		SlowWritesTotal:    d.slowWritesTotal.Load(),
		WritesOver3sTotal:  d.writesOver3sTotal.Load(),
		WritesOver10sTotal: d.writesOver10sTotal.Load(),
		MaxWriteMS:         d.maxWriteMS.Load(),
		ReadWaitsTotal:     d.readWaitsTotal.Load(),
		ReadWaitTotalMS:    d.readWaitTotalMS.Load(),
		ReadWaitsOver3s:    d.readWaitsOver3s.Load(),
		ReadWaitsOver10s:   d.readWaitsOver10s.Load(),
		MaxReadWaitMS:      d.maxReadWaitMS.Load(),
	}
}

func durationSinceStartMS(startedUnixNano, eventUnixNano int64) int64 {
	if eventUnixNano == 0 {
		return -1
	}

	return time.Duration(eventUnixNano - startedUnixNano).Milliseconds()
}

func durationSinceEventMS(now time.Time, eventUnixNano int64) int64 {
	if eventUnixNano == 0 {
		return -1
	}

	return now.Sub(time.Unix(0, eventUnixNano)).Milliseconds()
}

func bytesPerSecond(bytesWritten int64, elapsed time.Duration) int64 {
	if elapsed <= 0 {
		return 0
	}

	return int64(float64(bytesWritten) / elapsed.Seconds())
}

func resetStreamDeliveryForTest() {
	streamDeliveries.mu.Lock()
	streamDeliveries.active = make(map[uint64]*streamDelivery)
	streamDeliveries.mu.Unlock()
	streamDeliveries.bytesWrittenTotal.Store(0)
	streamDeliveries.writeCallsTotal.Store(0)
	streamDeliveries.slowWritesTotal.Store(0)
	streamDeliveries.writesOver3sTotal.Store(0)
	streamDeliveries.writesOver10sTotal.Store(0)
	streamDeliveries.maxWriteMS.Store(0)
	streamDeliveryID.Store(0)
}
