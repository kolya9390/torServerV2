package torr

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"server/settings"
)

func resetAdmissionCounters() {
	streamActive.Store(0)
	streamWaiting.Store(0)
	for {
		select {
		case <-slotReleased:
		default:
			return
		}
	}
}

func TestTryAcquireStreamSlotRejectWhenQueueFull(t *testing.T) {
	resetAdmissionCounters()
	settings.BTsets = &settings.BTSets{
		MaxConcurrentStreams: 1,
		StreamQueueSize:      0,
		StreamQueueWaitSec:   1,
	}

	release, err := tryAcquireStreamSlot(context.Background())
	if err != nil {
		t.Fatalf("unexpected acquire error: %v", err)
	}
	defer release()

	_, reject := tryAcquireStreamSlot(context.Background())
	if reject == nil {
		t.Fatalf("expected reject while queue is full")
	}
	if reject.RetryAfterSec < 1 {
		t.Fatalf("expected retry-after >= 1, got %d", reject.RetryAfterSec)
	}
}

func TestTryAcquireStreamSlotWaitsAndAcquires(t *testing.T) {
	resetAdmissionCounters()
	settings.BTsets = &settings.BTSets{
		MaxConcurrentStreams: 1,
		StreamQueueSize:      2,
		StreamQueueWaitSec:   2,
	}

	release, err := tryAcquireStreamSlot(context.Background())
	if err != nil {
		t.Fatalf("unexpected acquire error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(60 * time.Millisecond)
		release()
	}()

	waitRelease, waitErr := tryAcquireStreamSlot(context.Background())
	if waitErr != nil {
		t.Fatalf("expected queued request to acquire, got %v", waitErr)
	}
	waitRelease()
	<-done
}

func TestTryAcquireStreamSlotBurstIsBounded(t *testing.T) {
	resetAdmissionCounters()
	settings.BTsets = &settings.BTSets{
		MaxConcurrentStreams: 2,
		StreamQueueSize:      3,
		StreamQueueWaitSec:   1,
	}

	var accepted atomic.Int32
	var rejected atomic.Int32
	var wg sync.WaitGroup

	start := make(chan struct{})
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			release, err := tryAcquireStreamSlot(context.Background())
			if err != nil {
				rejected.Add(1)
				return
			}
			accepted.Add(1)
			time.Sleep(25 * time.Millisecond)
			release()
		}()
	}
	close(start)
	wg.Wait()

	if accepted.Load() == 0 {
		t.Fatalf("expected at least one accepted stream")
	}
	if rejected.Load() == 0 {
		t.Fatalf("expected some requests to be rejected under burst")
	}
	if waiting := streamWaiting.Load(); waiting != 0 {
		t.Fatalf("expected no waiters after burst, got %d", waiting)
	}
	if active := streamActive.Load(); active != 0 {
		t.Fatalf("expected no active streams after burst, got %d", active)
	}
}
