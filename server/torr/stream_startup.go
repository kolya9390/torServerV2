package torr

import (
	"sync"
	"time"

	"server/torr/storage/torrstor"
)

type streamStartupStateProvider func() streamStartupCacheSnapshot

type streamStartupCacheSnapshot struct {
	CacheCapacityBytes int64 `json:"cache_capacity_bytes"`
	CacheFilledBytes   int64 `json:"cache_filled_bytes"`
	ResidentPieces     int64 `json:"resident_pieces"`
	ReaderOffset       int64 `json:"reader_offset"`
	ReaderReadahead    int64 `json:"reader_readahead"`
}

type streamStartupReadWaitSnapshot struct {
	ElapsedMS      int64                      `json:"elapsed_ms"`
	WaitMS         int64                      `json:"wait_ms"`
	Offset         int64                      `json:"offset"`
	RequestedBytes int64                      `json:"requested_bytes"`
	State          streamStartupCacheSnapshot `json:"state"`
}

type streamStartupSnapshot struct {
	WarmupEligible bool                           `json:"warmup_eligible"`
	SkipReason     string                         `json:"skip_reason,omitempty"`
	TargetBytes    int64                          `json:"target_bytes"`
	ReadBytes      int64                          `json:"read_bytes"`
	ElapsedMS      int64                          `json:"warmup_elapsed_ms"`
	CompletedMS    int64                          `json:"warmup_completed_ms"`
	Outcome        string                         `json:"outcome"`
	OffsetRestored bool                           `json:"offset_restored"`
	AtStart        streamStartupCacheSnapshot     `json:"at_start"`
	AtCompletion   streamStartupCacheSnapshot     `json:"at_completion"`
	FirstReadWait  *streamStartupReadWaitSnapshot `json:"first_post_warmup_read_wait,omitempty"`
}

type streamStartupTimeline struct {
	mu sync.RWMutex

	startedUnixNano int64
	stateProvider   streamStartupStateProvider
	snapshot        streamStartupSnapshot
	warmupFinished  bool
}

func streamStartupStateProviderFor(cache *torrstor.Cache, reader *torrstor.Reader) streamStartupStateProvider {
	return func() streamStartupCacheSnapshot {
		return streamStartupCacheSnapshot{
			CacheCapacityBytes: cache.GetCapacity(),
			CacheFilledBytes:   cache.Filled(),
			ResidentPieces:     cache.ResidentPieces(),
			ReaderOffset:       reader.Offset(),
			ReaderReadahead:    reader.Readahead(),
		}
	}
}

func newStreamStartupTimeline(
	started time.Time,
	stateProvider streamStartupStateProvider,
) streamStartupTimeline {
	return streamStartupTimeline{
		startedUnixNano: started.UnixNano(),
		stateProvider:   stateProvider,
		snapshot: streamStartupSnapshot{
			CompletedMS: -1,
			Outcome:     "pending",
		},
	}
}

func (s *streamStartupTimeline) recordSkipped(reason string, targetBytes int64) {
	if s == nil {
		return
	}

	state := s.captureState()

	s.mu.Lock()
	s.snapshot.WarmupEligible = false
	s.snapshot.SkipReason = reason
	s.snapshot.TargetBytes = targetBytes
	s.snapshot.Outcome = "skipped"
	s.snapshot.CompletedMS = 0
	s.snapshot.AtCompletion = state
	s.warmupFinished = true
	s.mu.Unlock()
}

func (s *streamStartupTimeline) recordWarmupStarted(targetBytes int64) {
	if s == nil {
		return
	}

	state := s.captureState()

	s.mu.Lock()
	s.snapshot.WarmupEligible = true
	s.snapshot.TargetBytes = targetBytes
	s.snapshot.AtStart = state
	s.snapshot.Outcome = "running"
	s.mu.Unlock()
}

func (s *streamStartupTimeline) recordWarmupCompleted(result playbackStartupWarmupResult) {
	if s == nil {
		return
	}

	state := s.captureState()

	s.mu.Lock()
	s.snapshot.ReadBytes = result.readBytes
	s.snapshot.ElapsedMS = result.elapsed.Milliseconds()
	s.snapshot.CompletedMS = time.Since(time.Unix(0, s.startedUnixNano)).Milliseconds()
	s.snapshot.Outcome = result.outcome
	s.snapshot.OffsetRestored = result.offsetRestored
	s.snapshot.AtCompletion = state
	s.warmupFinished = true
	s.mu.Unlock()
}

func (s *streamStartupTimeline) recordFirstReadWait(wait time.Duration, offset int64, requestedBytes int) {
	if s == nil {
		return
	}

	s.mu.RLock()
	eligible := s.warmupFinished && s.snapshot.FirstReadWait == nil
	s.mu.RUnlock()

	if !eligible {
		return
	}

	state := s.captureState()
	waitSnapshot := &streamStartupReadWaitSnapshot{
		ElapsedMS:      time.Since(time.Unix(0, s.startedUnixNano)).Milliseconds(),
		WaitMS:         wait.Milliseconds(),
		Offset:         offset,
		RequestedBytes: int64(requestedBytes),
		State:          state,
	}

	s.mu.Lock()
	if s.warmupFinished && s.snapshot.FirstReadWait == nil {
		s.snapshot.FirstReadWait = waitSnapshot
	}
	s.mu.Unlock()
}

func (s *streamStartupTimeline) getSnapshot() streamStartupSnapshot {
	if s == nil {
		return streamStartupSnapshot{CompletedMS: -1, Outcome: "unavailable"}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.snapshot
}

func (s *streamStartupTimeline) captureState() streamStartupCacheSnapshot {
	if s == nil || s.stateProvider == nil {
		return streamStartupCacheSnapshot{}
	}

	return s.stateProvider()
}
