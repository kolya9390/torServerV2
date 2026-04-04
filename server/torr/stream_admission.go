package torr

import (
	"context"
	"runtime"
	"sync/atomic"
	"time"

	"server/settings"
)

var (
	streamActive  atomic.Int64
	streamWaiting atomic.Int64
	// slotReleased is a best-effort signal that capacity may be available again.
	// It reduces busy polling under high contention.
	slotReleased = make(chan struct{}, 1024)
)

type StreamAdmissionError struct {
	Reason        string
	RetryAfterSec int
}

func (e *StreamAdmissionError) Error() string {
	return e.Reason
}

type streamAdmissionConfig struct {
	maxStreams int64
	queueSize  int64
	wait       time.Duration
}

type StreamAdmissionSnapshot struct {
	Active     int64
	Waiting    int64
	MaxStreams int64
	QueueSize  int64
}

func currentAdmissionConfig() streamAdmissionConfig {
	maxStreams := int64(runtime.GOMAXPROCS(0) * 2)
	queueSize := maxStreams * 2
	wait := 3 * time.Second

	if settings.BTsets != nil {
		if settings.BTsets.MaxConcurrentStreams > 0 {
			maxStreams = int64(settings.BTsets.MaxConcurrentStreams)
		}
		if settings.BTsets.StreamQueueSize > 0 {
			queueSize = int64(settings.BTsets.StreamQueueSize)
		}
		if settings.BTsets.StreamQueueWaitSec > 0 {
			wait = time.Duration(settings.BTsets.StreamQueueWaitSec) * time.Second
		}
	}
	if maxStreams < 1 {
		maxStreams = 1
	}
	if queueSize < 0 {
		queueSize = 0
	}
	return streamAdmissionConfig{
		maxStreams: maxStreams,
		queueSize:  queueSize,
		wait:       wait,
	}
}

func tryAcquireStreamSlot(ctx context.Context) (func(), *StreamAdmissionError) {
	cfg := currentAdmissionConfig()
	if claimActiveSlot(cfg.maxStreams) {
		return releaseStreamSlot, nil
	}

	waitingNow := streamWaiting.Add(1)
	queued := true
	defer func() {
		if queued {
			streamWaiting.Add(-1)
		}
	}()

	if waitingNow > cfg.queueSize {
		return nil, &StreamAdmissionError{
			Reason:        "stream queue is full",
			RetryAfterSec: recommendedRetryAfter(cfg.maxStreams),
		}
	}

	waitCtx := ctx
	cancel := func() {}
	if cfg.wait > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, cfg.wait)
	}
	defer cancel()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		if claimActiveSlot(cfg.maxStreams) {
			streamWaiting.Add(-1)
			queued = false
			return releaseStreamSlot, nil
		}
		select {
		case <-waitCtx.Done():
			return nil, &StreamAdmissionError{
				Reason:        "stream capacity reached, retry later",
				RetryAfterSec: recommendedRetryAfter(cfg.maxStreams),
			}
		case <-slotReleased:
		case <-ticker.C:
		}
	}
}

func claimActiveSlot(maxStreams int64) bool {
	for {
		active := streamActive.Load()
		if active >= maxStreams {
			return false
		}
		if streamActive.CompareAndSwap(active, active+1) {
			return true
		}
	}
}

func releaseStreamSlot() {
	if streamActive.Add(-1) < 0 {
		streamActive.Store(0)
	}
	select {
	case slotReleased <- struct{}{}:
	default:
	}
}

func recommendedRetryAfter(maxStreams int64) int {
	retry := 1 + int((streamWaiting.Load()+maxStreams-1)/maxStreams)
	if retry < 1 {
		return 1
	}
	if retry > 10 {
		return 10
	}
	return retry
}

func GetActiveStreams() int32 {
	return int32(streamActive.Load())
}

func GetStreamAdmissionSnapshot() StreamAdmissionSnapshot {
	cfg := currentAdmissionConfig()
	return StreamAdmissionSnapshot{
		Active:     streamActive.Load(),
		Waiting:    streamWaiting.Load(),
		MaxStreams: cfg.maxStreams,
		QueueSize:  cfg.queueSize,
	}
}
