package torr

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"time"

	"server/settings"
)

// activeStreams counts currently active streaming connections.
var activeStreams int32
var lastStreamActivityUnixNano int64

// streamAdmission controls concurrent stream limiting.
type streamAdmission struct {
	maxStreams   int32
	waitDuration time.Duration
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

	return streamAdmission{
		maxStreams:   int32(maxStreams),
		waitDuration: time.Duration(waitSec) * time.Second,
	}
}

func acquireStreamSlot(maxStreams int32) bool {
	for {
		current := atomic.LoadInt32(&activeStreams)
		if current >= maxStreams {
			return false
		}

		if atomic.CompareAndSwapInt32(&activeStreams, current, current+1) {
			return true
		}
	}
}

func markStreamActivity() {
	atomic.StoreInt64(&lastStreamActivityUnixNano, time.Now().UnixNano())
}

func tryAcquireStream(ctx context.Context, sets *settings.BTSets) (func(), error) {
	admission := currentAdmission(sets)

	if !acquireStreamSlot(admission.maxStreams) {
		deadline := time.After(admission.waitDuration)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()

	waitLoop:
		for {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-deadline:
				return nil, errors.New("stream limit exceeded, try again later")
			case <-ticker.C:
				if acquireStreamSlot(admission.maxStreams) {
					break waitLoop
				}
			}
		}
	}

	release := func() {
		if atomic.AddInt32(&activeStreams, -1) < 0 {
			atomic.StoreInt32(&activeStreams, 0)
		}
	}

	return release, nil
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
