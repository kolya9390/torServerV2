package torr

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"server/settings"
)

// activeStreams counts currently active streaming connections.
var activeStreams int32
var lastStreamActivityUnixNano int64
var streamAdmissionState = newStreamAdmissionState()

const streamAdmissionSnapshotLimit = 64

// streamAdmission controls concurrent stream limiting.
type streamAdmission struct {
	maxStreams        int32
	maxUniqueTorrents int
	queueSize         int
	waitDuration      time.Duration
}

type streamAdmissionRegistry struct {
	mu            sync.Mutex
	active        map[string]streamAdmissionActive
	waiters       int
	uniqueWaiters int

	waitsTotal              atomic.Int64
	uniqueWaitsTotal        atomic.Int64
	timeoutsTotal           atomic.Int64
	overloadRejectionsTotal atomic.Int64
}

type streamAdmissionActive struct {
	readers   int
	torrentID uint64
}

type streamAdmissionSnapshot struct {
	ActiveStreams                int32                           `json:"active_streams"`
	ActiveUniquePlaybackTorrents int                             `json:"active_unique_playback_torrents"`
	ExtraSameTorrentStreams      int                             `json:"extra_same_torrent_streams"`
	MaxReadersPerTorrent         int                             `json:"max_readers_per_torrent"`
	QueuedRequests               int                             `json:"queued_requests"`
	QueuedUniqueTorrentRequests  int                             `json:"queued_unique_torrent_requests"`
	AdmissionWaitsTotal          int64                           `json:"admission_waits_total"`
	UniqueTorrentWaitsTotal      int64                           `json:"unique_torrent_waits_total"`
	AdmissionTimeoutsTotal       int64                           `json:"admission_timeouts_total"`
	OverloadRejectionsTotal      int64                           `json:"overload_rejections_total"`
	Streams                      []activeStreamAdmissionSnapshot `json:"streams"`
}

type activeStreamAdmissionSnapshot struct {
	TorrentID uint64 `json:"torrent_id"`
	Readers   int    `json:"readers"`
}

func currentAdmission(sets *settings.BTSets) streamAdmission {
	if sets == nil {
		sets = &settings.BTSets{}
	}

	streamCfg := sets.StreamConfig()

	maxStreams := streamCfg.MaxConcurrentStreams
	if maxStreams <= 0 {
		maxStreams = maxInt(1, runtime.GOMAXPROCS(0)*2)
	}

	waitSec := streamCfg.StreamQueueWaitSec
	if waitSec <= 0 {
		waitSec = 3
	}

	queueSize := streamCfg.StreamQueueSize
	if queueSize <= 0 {
		queueSize = maxInt(1, int(maxStreams)*2)
	}

	return streamAdmission{
		maxStreams:        int32(maxStreams),
		maxUniqueTorrents: maxInt(streamCfg.MaxUniquePlaybackTorrents, 0),
		queueSize:         queueSize,
		waitDuration:      time.Duration(waitSec) * time.Second,
	}
}

func newStreamAdmissionState() *streamAdmissionRegistry {
	return &streamAdmissionRegistry{
		active: make(map[string]streamAdmissionActive),
	}
}

func markStreamActivity() {
	atomic.StoreInt64(&lastStreamActivityUnixNano, time.Now().UnixNano())
}

func tryAcquireStream(
	ctx context.Context,
	sets *settings.BTSets,
	torrentKey string,
	torrentID uint64,
	debugEnabled bool,
) (func(), error) {
	admission := currentAdmission(sets)
	queued := false
	queuedUnique := false
	timer := time.NewTimer(admission.waitDuration)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer timer.Stop()
	defer ticker.Stop()

	for {
		release, ok, uniqueBlocked := streamAdmissionState.tryAcquire(admission, torrentKey, torrentID)
		if ok {
			if queued {
				streamAdmissionState.releaseQueueSlot(queuedUnique, debugEnabled)
			}

			return release, nil
		}

		if !queued {
			if !streamAdmissionState.tryAcquireQueueSlot(admission.queueSize, uniqueBlocked, debugEnabled) {
				streamAdmissionState.recordOverloadRejection(debugEnabled)

				return nil, errors.New("stream queue is full, try again later")
			}

			queued = true
			queuedUnique = uniqueBlocked
		}

		select {
		case <-ctx.Done():
			streamAdmissionState.releaseQueueSlot(queuedUnique, debugEnabled)

			return nil, ctx.Err()
		case <-timer.C:
			streamAdmissionState.releaseQueueSlot(queuedUnique, debugEnabled)
			streamAdmissionState.recordAdmissionTimeout(debugEnabled)

			return nil, errors.New("stream limit exceeded, try again later")
		case <-ticker.C:
		}
	}
}

func (r *streamAdmissionRegistry) tryAcquire(
	admission streamAdmission,
	torrentKey string,
	torrentID uint64,
) (func(), bool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if atomic.LoadInt32(&activeStreams) >= admission.maxStreams {
		return nil, false, false
	}

	if !r.canAcquireUniqueTorrent(admission.maxUniqueTorrents, torrentKey) {
		return nil, false, true
	}

	atomic.AddInt32(&activeStreams, 1)
	if torrentKey != "" {
		active := r.active[torrentKey]
		active.readers++
		if active.torrentID == 0 {
			active.torrentID = torrentID
		}

		r.active[torrentKey] = active
	}

	var once sync.Once
	release := func() {
		once.Do(func() {
			r.release(torrentKey)
		})
	}

	return release, true, false
}

func (r *streamAdmissionRegistry) canAcquireUniqueTorrent(maxUnique int, torrentKey string) bool {
	if maxUnique <= 0 || torrentKey == "" {
		return true
	}

	if r.active[torrentKey].readers > 0 {
		return true
	}

	return len(r.active) < maxUnique
}

func (r *streamAdmissionRegistry) release(torrentKey string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if torrentKey != "" {
		count := r.active[torrentKey]
		if count.readers <= 1 {
			delete(r.active, torrentKey)
		} else {
			count.readers--
			r.active[torrentKey] = count
		}
	}

	if atomic.AddInt32(&activeStreams, -1) < 0 {
		atomic.StoreInt32(&activeStreams, 0)
	}
}

func (r *streamAdmissionRegistry) tryAcquireQueueSlot(queueSize int, uniqueBlocked, debugEnabled bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if queueSize <= 0 {
		return false
	}

	if r.waiters >= queueSize {
		return false
	}

	r.waiters++
	if debugEnabled {
		r.waitsTotal.Add(1)
		if uniqueBlocked {
			r.uniqueWaiters++
			r.uniqueWaitsTotal.Add(1)
		}
	}

	return true
}

func (r *streamAdmissionRegistry) releaseQueueSlot(uniqueWaiter, debugEnabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.waiters > 0 {
		r.waiters--
	}

	if debugEnabled && uniqueWaiter && r.uniqueWaiters > 0 {
		r.uniqueWaiters--
	}
}

func (r *streamAdmissionRegistry) recordAdmissionTimeout(debugEnabled bool) {
	if debugEnabled {
		r.timeoutsTotal.Add(1)
	}
}

func (r *streamAdmissionRegistry) recordOverloadRejection(debugEnabled bool) {
	if debugEnabled {
		r.overloadRejectionsTotal.Add(1)
	}
}

func SnapshotStreamAdmission() streamAdmissionSnapshot {
	streamAdmissionState.mu.Lock()
	defer streamAdmissionState.mu.Unlock()

	snapshot := streamAdmissionSnapshot{
		ActiveStreams:                GetActiveStreams(),
		ActiveUniquePlaybackTorrents: len(streamAdmissionState.active),
		QueuedRequests:               streamAdmissionState.waiters,
		QueuedUniqueTorrentRequests:  streamAdmissionState.uniqueWaiters,
		AdmissionWaitsTotal:          streamAdmissionState.waitsTotal.Load(),
		UniqueTorrentWaitsTotal:      streamAdmissionState.uniqueWaitsTotal.Load(),
		AdmissionTimeoutsTotal:       streamAdmissionState.timeoutsTotal.Load(),
		OverloadRejectionsTotal:      streamAdmissionState.overloadRejectionsTotal.Load(),
		Streams:                      make([]activeStreamAdmissionSnapshot, 0),
	}

	for _, active := range streamAdmissionState.active {
		if len(snapshot.Streams) >= streamAdmissionSnapshotLimit {
			break
		}

		if active.readers > snapshot.MaxReadersPerTorrent {
			snapshot.MaxReadersPerTorrent = active.readers
		}

		snapshot.Streams = append(snapshot.Streams, activeStreamAdmissionSnapshot{
			TorrentID: active.torrentID,
			Readers:   active.readers,
		})
	}

	snapshot.ExtraSameTorrentStreams = int(snapshot.ActiveStreams) - snapshot.ActiveUniquePlaybackTorrents

	return snapshot
}

// GetActiveStreams returns number of currently active streams.
func GetActiveStreams() int32 {
	return atomic.LoadInt32(&activeStreams)
}

// SinceLastStreamActivity returns time elapsed since the last stream activity event.
func SinceLastStreamActivity() time.Duration {
	ns := atomic.LoadInt64(&lastStreamActivityUnixNano)
	if ns == 0 {
		return time.Duration(1<<63 - 1)
	}

	return time.Since(time.Unix(0, ns))
}
