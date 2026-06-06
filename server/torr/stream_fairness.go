package torr

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	streamFairnessMinActiveStreams = 2
	streamFairnessMinAge           = 10 * time.Second
	streamFairnessMinBytes         = int64(16 << 20)
	streamFairnessFastBPS          = int64(6 << 20)
	streamFairnessDelayCooldown    = 5 * time.Millisecond
	streamFairnessBaseDelay        = 250 * time.Microsecond
	streamFairnessMaxDelay         = 750 * time.Microsecond
	streamFairnessSnapshotLimit    = 64

	streamFairnessImbalanceNumerator   = int64(3)
	streamFairnessImbalanceDenominator = int64(2)
	streamFairnessSevereNumerator      = int64(2)
	streamFairnessSevereDenominator    = int64(1)
)

var (
	streamFairnessID atomic.Uint64
	streamFairness   = streamFairnessRegistry{
		active: make(map[uint64]*streamFairnessFlow),
		sleep:  time.Sleep,
	}
)

type streamFairnessRegistry struct {
	mu sync.RWMutex

	active map[uint64]*streamFairnessFlow
	sleep  func(time.Duration)

	delayedWritesTotal atomic.Int64
	delayTotalMicros   atomic.Int64
	maxDelayMicros     atomic.Int64
}

type streamFairnessFlow struct {
	id uint64

	torrentID        uint64
	startedUnixNano  int64
	lastDelayNano    atomic.Int64
	bytesWritten     atomic.Int64
	writeCalls       atomic.Int64
	delayedWrites    atomic.Int64
	delayTotalMicros atomic.Int64
	maxDelayMicros   atomic.Int64
}

type streamFairnessSnapshot struct {
	ActiveStreams      int                          `json:"active_streams"`
	DelayedWritesTotal int64                        `json:"delayed_writes_total"`
	DelayTotalMicros   int64                        `json:"delay_total_us"`
	MaxDelayMicros     int64                        `json:"max_delay_us"`
	Streams            []activeStreamFairnessRecord `json:"streams"`
}

type activeStreamFairnessRecord struct {
	TorrentID          uint64 `json:"torrent_id"`
	ID                 uint64 `json:"id"`
	ElapsedMS          int64  `json:"elapsed_ms"`
	BytesWritten       int64  `json:"bytes_written"`
	BytesPerSecond     int64  `json:"bytes_per_second"`
	WriteCalls         int64  `json:"write_calls"`
	DelayedWritesTotal int64  `json:"delayed_writes_total"`
	DelayTotalMicros   int64  `json:"delay_total_us"`
	MaxDelayMicros     int64  `json:"max_delay_us"`
}

func registerStreamFairness(started time.Time, torrentID uint64) (*streamFairnessFlow, func()) {
	flow := &streamFairnessFlow{
		id:              streamFairnessID.Add(1),
		torrentID:       torrentID,
		startedUnixNano: started.UnixNano(),
	}

	streamFairness.mu.Lock()
	streamFairness.active[flow.id] = flow
	streamFairness.mu.Unlock()

	return flow, func() {
		streamFairness.mu.Lock()
		delete(streamFairness.active, flow.id)
		streamFairness.mu.Unlock()
	}
}

func (f *streamFairnessFlow) recordWriteAndMaybeDelay(bytesWritten int) {
	if f == nil || bytesWritten <= 0 {
		return
	}

	f.writeCalls.Add(1)
	f.bytesWritten.Add(int64(bytesWritten))

	if delay := f.fairnessDelay(time.Now()); delay > 0 {
		f.recordFairnessDelay(delay)
		streamFairness.sleep(delay)
	}
}

func (f *streamFairnessFlow) fairnessDelay(now time.Time) time.Duration {
	streamFairness.mu.RLock()
	defer streamFairness.mu.RUnlock()

	if len(streamFairness.active) < streamFairnessMinActiveStreams {
		return 0
	}

	currentBPS, ok := f.matureBytesPerSecond(now)
	if !ok || currentBPS < streamFairnessFastBPS {
		return 0
	}

	mature := 0
	minBPS := int64(0)

	for _, flow := range streamFairness.active {
		bps, matureFlow := flow.matureBytesPerSecond(now)
		if !matureFlow {
			continue
		}

		mature++

		if minBPS == 0 || bps < minBPS {
			minBPS = bps
		}
	}

	if mature < streamFairnessMinActiveStreams || minBPS <= 0 {
		return 0
	}

	if currentBPS*streamFairnessImbalanceDenominator <= minBPS*streamFairnessImbalanceNumerator {
		return 0
	}

	lastDelay := f.lastDelayNano.Load()
	if lastDelay != 0 && now.Sub(time.Unix(0, lastDelay)) < streamFairnessDelayCooldown {
		return 0
	}

	if !f.lastDelayNano.CompareAndSwap(lastDelay, now.UnixNano()) {
		return 0
	}

	if currentBPS*streamFairnessSevereDenominator > minBPS*streamFairnessSevereNumerator {
		return streamFairnessMaxDelay
	}

	return streamFairnessBaseDelay
}

func (f *streamFairnessFlow) matureBytesPerSecond(now time.Time) (int64, bool) {
	elapsed := now.Sub(time.Unix(0, f.startedUnixNano))
	bytesWritten := f.bytesWritten.Load()

	if elapsed < streamFairnessMinAge || bytesWritten < streamFairnessMinBytes {
		return 0, false
	}

	return bytesPerSecond(bytesWritten, elapsed), true
}

func (f *streamFairnessFlow) recordFairnessDelay(delay time.Duration) {
	delayMicros := delay.Microseconds()

	f.delayedWrites.Add(1)
	f.delayTotalMicros.Add(delayMicros)
	updateMaxAtomic(&f.maxDelayMicros, delayMicros)

	streamFairness.delayedWritesTotal.Add(1)
	streamFairness.delayTotalMicros.Add(delayMicros)
	updateMaxAtomic(&streamFairness.maxDelayMicros, delayMicros)
}

func SnapshotStreamFairness() streamFairnessSnapshot {
	now := time.Now()
	snapshot := streamFairnessSnapshot{
		DelayedWritesTotal: streamFairness.delayedWritesTotal.Load(),
		DelayTotalMicros:   streamFairness.delayTotalMicros.Load(),
		MaxDelayMicros:     streamFairness.maxDelayMicros.Load(),
		Streams:            make([]activeStreamFairnessRecord, 0),
	}

	streamFairness.mu.RLock()
	defer streamFairness.mu.RUnlock()

	snapshot.ActiveStreams = len(streamFairness.active)
	snapshot.Streams = make(
		[]activeStreamFairnessRecord,
		0,
		min(len(streamFairness.active), streamFairnessSnapshotLimit),
	)

	for _, flow := range streamFairness.active {
		if len(snapshot.Streams) >= streamFairnessSnapshotLimit {
			break
		}

		snapshot.Streams = append(snapshot.Streams, flow.fairnessSnapshot(now))
	}

	return snapshot
}

func (f *streamFairnessFlow) fairnessSnapshot(now time.Time) activeStreamFairnessRecord {
	elapsed := now.Sub(time.Unix(0, f.startedUnixNano))
	bytesWritten := f.bytesWritten.Load()

	return activeStreamFairnessRecord{
		TorrentID:          f.torrentID,
		ID:                 f.id,
		ElapsedMS:          elapsed.Milliseconds(),
		BytesWritten:       bytesWritten,
		BytesPerSecond:     bytesPerSecond(bytesWritten, elapsed),
		WriteCalls:         f.writeCalls.Load(),
		DelayedWritesTotal: f.delayedWrites.Load(),
		DelayTotalMicros:   f.delayTotalMicros.Load(),
		MaxDelayMicros:     f.maxDelayMicros.Load(),
	}
}

func resetStreamFairnessForTest() {
	streamFairness.mu.Lock()
	streamFairness.active = make(map[uint64]*streamFairnessFlow)
	streamFairness.sleep = time.Sleep
	streamFairness.mu.Unlock()

	streamFairness.delayedWritesTotal.Store(0)
	streamFairness.delayTotalMicros.Store(0)
	streamFairness.maxDelayMicros.Store(0)
	streamFairnessID.Store(0)
}
