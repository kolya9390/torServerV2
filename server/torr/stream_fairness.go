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
	streamFairnessFastBPS          = int64(2 << 20)
	streamFairnessDelayCooldown    = 5 * time.Millisecond
	streamFairnessBaseDelay        = 250 * time.Microsecond
	streamFairnessMaxDelay         = 750 * time.Microsecond
	streamStartupProtectionWindow  = 20 * time.Second
	streamStartupProtectionDelay   = 500 * time.Microsecond
	streamStartupProtectionMinGap  = 2 * time.Second
	streamFairnessSnapshotLimit    = 64
	streamFairnessPressureWindow   = 10 * time.Second
	streamFairnessSevereReadWait   = 3 * time.Second

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

	startupProtectedWritesTotal  atomic.Int64
	startupProtectionDelayMicros atomic.Int64
}

type streamFairnessFlow struct {
	id uint64

	torrentID        uint64
	startedUnixNano  int64
	delivery         atomic.Pointer[streamDelivery]
	lastDelayNano    atomic.Int64
	bytesWritten     atomic.Int64
	writeCalls       atomic.Int64
	delayedWrites    atomic.Int64
	delayTotalMicros atomic.Int64
	maxDelayMicros   atomic.Int64

	startupProtectedWrites atomic.Int64
}

type streamFairnessSnapshot struct {
	ActiveStreams           int                          `json:"active_streams"`
	ActiveUniqueTorrents    int                          `json:"active_unique_torrents"`
	ExtraSameTorrentStreams int                          `json:"extra_same_torrent_streams"`
	DelayedWritesTotal      int64                        `json:"delayed_writes_total"`
	DelayTotalMicros        int64                        `json:"delay_total_us"`
	MaxDelayMicros          int64                        `json:"max_delay_us"`
	StartupProtectedWrites  int64                        `json:"startup_protected_writes_total"`
	StartupProtectionDelay  int64                        `json:"startup_protection_delay_us"`
	StartupProtectionActive int                          `json:"startup_protection_active_torrents"`
	StartupProtectionWindow int64                        `json:"startup_protection_window_ms"`
	Streams                 []activeStreamFairnessRecord `json:"streams"`
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
	StartupProtected   int64  `json:"startup_protected_writes_total"`
}

type streamFairnessTorrentState struct {
	torrentID             uint64
	startedUnixNano       int64
	bytesWritten          int64
	activeStreams         int
	hasFirstByte          bool
	hasRecentReadWait     bool
	hasClientBackpressure bool
	hasSevereReadWait     bool
}

type streamFairnessDelayDecision struct {
	duration          time.Duration
	startupProtection bool
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

	if decision := f.fairnessDelay(time.Now()); decision.duration > 0 {
		f.recordFairnessDelay(decision)
		streamFairness.sleep(decision.duration)
	}
}

func (f *streamFairnessFlow) setDelivery(delivery *streamDelivery) {
	if f != nil && delivery != nil {
		f.delivery.Store(delivery)
	}
}

func (f *streamFairnessFlow) fairnessDelay(now time.Time) streamFairnessDelayDecision {
	streamFairness.mu.RLock()
	defer streamFairness.mu.RUnlock()

	if len(streamFairness.active) < streamFairnessMinActiveStreams {
		return streamFairnessDelayDecision{}
	}

	if f.torrentID == 0 {
		return streamFairnessDelayDecision{}
	}

	current, aggregates, ok := f.fairnessCurrent(now)
	if !ok {
		return streamFairnessDelayDecision{}
	}

	currentBPS, ok := current.matureBytesPerSecond(now)
	if !ok || currentBPS < streamFairnessFastBPS {
		return streamFairnessDelayDecision{}
	}

	if current.hasSevereReadWait || current.hasClientBackpressure {
		return streamFairnessDelayDecision{}
	}

	if startup := startupProtectionCandidate(aggregates, current, f.torrentID, now); startup != nil {
		return f.rateLimitedDelay(now, streamStartupProtectionDelay, true)
	}

	weakest := weakestMatureFairnessCandidate(aggregates, f.torrentID, now)
	if weakest == nil {
		return streamFairnessDelayDecision{}
	}

	minBPS, ok := weakest.matureBytesPerSecond(now)
	if !ok || minBPS <= 0 || weakest.hasClientBackpressure {
		return streamFairnessDelayDecision{}
	}

	if currentBPS*streamFairnessImbalanceDenominator <= minBPS*streamFairnessImbalanceNumerator {
		return streamFairnessDelayDecision{}
	}

	if currentBPS*streamFairnessSevereDenominator > minBPS*streamFairnessSevereNumerator {
		return f.rateLimitedDelay(now, streamFairnessMaxDelay, false)
	}

	return f.rateLimitedDelay(now, streamFairnessBaseDelay, false)
}

func (f *streamFairnessFlow) fairnessCurrent(
	now time.Time,
) (*streamFairnessTorrentState, map[uint64]*streamFairnessTorrentState, bool) {
	aggregates := streamFairnessTorrentAggregates(now)

	current := aggregates[f.torrentID]
	if current == nil {
		return nil, nil, false
	}

	return current, aggregates, len(aggregates) >= streamFairnessMinActiveStreams
}

func weakestMatureFairnessCandidate(
	aggregates map[uint64]*streamFairnessTorrentState,
	currentTorrentID uint64,
	now time.Time,
) *streamFairnessTorrentState {
	var weakest *streamFairnessTorrentState

	for torrentID, aggregate := range aggregates {
		if torrentID == currentTorrentID || !aggregate.isMature(now) {
			continue
		}

		if weakest == nil || aggregate.bytesPerSecond(now) < weakest.bytesPerSecond(now) {
			weakest = aggregate
		}
	}

	return weakest
}

func startupProtectionCandidate(
	aggregates map[uint64]*streamFairnessTorrentState,
	current *streamFairnessTorrentState,
	currentTorrentID uint64,
	now time.Time,
) *streamFairnessTorrentState {
	var candidate *streamFairnessTorrentState

	for torrentID, aggregate := range aggregates {
		if torrentID == currentTorrentID || !aggregate.needsStartupProtectionAfter(current, now) {
			continue
		}

		if candidate == nil || aggregate.startedUnixNano > candidate.startedUnixNano {
			candidate = aggregate
		}
	}

	return candidate
}

func (f *streamFairnessFlow) rateLimitedDelay(
	now time.Time,
	delay time.Duration,
	startupProtection bool,
) streamFairnessDelayDecision {
	lastDelay := f.lastDelayNano.Load()
	if lastDelay != 0 && now.Sub(time.Unix(0, lastDelay)) < streamFairnessDelayCooldown {
		return streamFairnessDelayDecision{}
	}

	if !f.lastDelayNano.CompareAndSwap(lastDelay, now.UnixNano()) {
		return streamFairnessDelayDecision{}
	}

	return streamFairnessDelayDecision{duration: delay, startupProtection: startupProtection}
}

func streamFairnessTorrentAggregates(now time.Time) map[uint64]*streamFairnessTorrentState {
	aggregates := make(map[uint64]*streamFairnessTorrentState)

	for _, flow := range streamFairness.active {
		if flow.torrentID == 0 {
			continue
		}

		aggregate := aggregates[flow.torrentID]
		if aggregate == nil {
			aggregate = &streamFairnessTorrentState{
				torrentID:       flow.torrentID,
				startedUnixNano: flow.startedUnixNano,
			}
			aggregates[flow.torrentID] = aggregate
		}

		aggregate.add(flow, now)
	}

	return aggregates
}

func (s *streamFairnessTorrentState) add(flow *streamFairnessFlow, now time.Time) {
	s.activeStreams++
	s.bytesWritten += flow.bytesWritten.Load()

	if flow.startedUnixNano < s.startedUnixNano {
		s.startedUnixNano = flow.startedUnixNano
	}

	s.hasFirstByte = s.hasFirstByte || flow.hasFirstByte()
	s.hasRecentReadWait = s.hasRecentReadWait || flow.hasRecentReadWait(now)
	s.hasClientBackpressure = s.hasClientBackpressure || flow.hasClientBackpressure()
	s.hasSevereReadWait = s.hasSevereReadWait || flow.hasRecentSevereReadWait(now)
}

func (s *streamFairnessTorrentState) needsStartupProtection(now time.Time) bool {
	if s == nil || s.hasClientBackpressure {
		return false
	}

	elapsed := now.Sub(time.Unix(0, s.startedUnixNano))
	if elapsed < 0 || elapsed > streamStartupProtectionWindow {
		return false
	}

	if s.isMature(now) {
		return false
	}

	return !s.hasFirstByte || s.hasRecentReadWait
}

func (s *streamFairnessTorrentState) needsStartupProtectionAfter(
	current *streamFairnessTorrentState,
	now time.Time,
) bool {
	if !s.needsStartupProtection(now) || current == nil {
		return false
	}

	return time.Unix(0, s.startedUnixNano).Sub(time.Unix(0, current.startedUnixNano)) >= streamStartupProtectionMinGap
}

func (s *streamFairnessTorrentState) isMature(now time.Time) bool {
	_, ok := s.matureBytesPerSecond(now)

	return ok
}

func (s *streamFairnessTorrentState) matureBytesPerSecond(now time.Time) (int64, bool) {
	if s == nil {
		return 0, false
	}

	elapsed := now.Sub(time.Unix(0, s.startedUnixNano))
	if elapsed < streamFairnessMinAge || s.bytesWritten < streamFairnessMinBytes {
		return 0, false
	}

	return bytesPerSecond(s.bytesWritten, elapsed), true
}

func (s *streamFairnessTorrentState) bytesPerSecond(now time.Time) int64 {
	if s == nil {
		return 0
	}

	elapsed := now.Sub(time.Unix(0, s.startedUnixNano))

	return bytesPerSecond(s.bytesWritten, elapsed)
}

func (f *streamFairnessFlow) hasFirstByte() bool {
	if f == nil {
		return false
	}

	if f.bytesWritten.Load() > 0 {
		return true
	}

	delivery := f.delivery.Load()
	if delivery == nil {
		return false
	}

	return delivery.firstWriteUnixNano.Load() > 0
}

func (f *streamFairnessFlow) hasRecentReadWait(now time.Time) bool {
	if f == nil {
		return false
	}

	delivery := f.delivery.Load()
	if delivery == nil || delivery.readWaitsTotal.Load() == 0 {
		return false
	}

	last := delivery.lastReadWaitNano.Load()
	if last == 0 {
		return false
	}

	return now.Sub(time.Unix(0, last)) <= streamFairnessPressureWindow
}

func (f *streamFairnessFlow) hasRecentSevereReadWait(now time.Time) bool {
	if f == nil {
		return false
	}

	delivery := f.delivery.Load()
	if delivery == nil || delivery.lastReadWaitMS.Load() <= streamFairnessSevereReadWait.Milliseconds() {
		return false
	}

	last := delivery.lastReadWaitNano.Load()
	if last == 0 {
		return false
	}

	return now.Sub(time.Unix(0, last)) <= streamFairnessPressureWindow
}

func (f *streamFairnessFlow) hasClientBackpressure() bool {
	if f == nil {
		return false
	}

	delivery := f.delivery.Load()
	if delivery == nil {
		return false
	}

	return delivery.slowWritesTotal.Load() > 0 ||
		delivery.writesOver3sTotal.Load() > 0 ||
		delivery.writesOver10sTotal.Load() > 0
}

func (f *streamFairnessFlow) recordFairnessDelay(decision streamFairnessDelayDecision) {
	delayMicros := decision.duration.Microseconds()

	f.delayedWrites.Add(1)
	f.delayTotalMicros.Add(delayMicros)
	updateMaxAtomic(&f.maxDelayMicros, delayMicros)

	streamFairness.delayedWritesTotal.Add(1)
	streamFairness.delayTotalMicros.Add(delayMicros)
	updateMaxAtomic(&streamFairness.maxDelayMicros, delayMicros)

	if decision.startupProtection {
		f.startupProtectedWrites.Add(1)
		streamFairness.startupProtectedWritesTotal.Add(1)
		streamFairness.startupProtectionDelayMicros.Add(delayMicros)
	}
}

func SnapshotStreamFairness() streamFairnessSnapshot {
	now := time.Now()
	snapshot := streamFairnessSnapshot{
		DelayedWritesTotal:      streamFairness.delayedWritesTotal.Load(),
		DelayTotalMicros:        streamFairness.delayTotalMicros.Load(),
		MaxDelayMicros:          streamFairness.maxDelayMicros.Load(),
		StartupProtectedWrites:  streamFairness.startupProtectedWritesTotal.Load(),
		StartupProtectionDelay:  streamFairness.startupProtectionDelayMicros.Load(),
		StartupProtectionWindow: streamStartupProtectionWindow.Milliseconds(),
		Streams:                 make([]activeStreamFairnessRecord, 0),
	}

	streamFairness.mu.RLock()
	defer streamFairness.mu.RUnlock()

	snapshot.ActiveStreams = len(streamFairness.active)
	aggregates := streamFairnessTorrentAggregates(now)
	snapshot.ActiveUniqueTorrents = len(aggregates)
	snapshot.ExtraSameTorrentStreams = snapshot.ActiveStreams - snapshot.ActiveUniqueTorrents
	snapshot.StartupProtectionActive = countStartupProtectedTorrents(aggregates, now)
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

func countStartupProtectedTorrents(aggregates map[uint64]*streamFairnessTorrentState, now time.Time) int {
	count := 0

	for _, aggregate := range aggregates {
		if aggregate.needsStartupProtection(now) {
			count++
		}
	}

	return count
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
		StartupProtected:   f.startupProtectedWrites.Load(),
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
	streamFairness.startupProtectedWritesTotal.Store(0)
	streamFairness.startupProtectionDelayMicros.Store(0)
	streamFairnessID.Store(0)
}
