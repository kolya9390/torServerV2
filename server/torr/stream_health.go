package torr

import (
	"errors"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	streamSlowFirstByteThreshold = 2 * time.Second
	streamReadWaitThreshold      = 500 * time.Millisecond
)

var streamHealth streamHealthCounters

type streamHealthCounters struct {
	requestsTotal          atomic.Int64
	slowFirstByteTotal     atomic.Int64
	zeroByteResponsesTotal atomic.Int64
	clientDisconnectsTotal atomic.Int64
	stallsTotal            atomic.Int64
	bytesWrittenTotal      atomic.Int64
	firstByteBuckets       [6]atomic.Int64
	readWaitBuckets        [6]atomic.Int64
}

type streamHealthSnapshot struct {
	RequestsTotal          int64            `json:"requests_total"`
	SlowFirstByteTotal     int64            `json:"slow_first_byte_total"`
	ZeroByteResponsesTotal int64            `json:"zero_byte_responses_total"`
	ClientDisconnectsTotal int64            `json:"client_disconnects_total"`
	StallsTotal            int64            `json:"stalls_total"`
	BytesWrittenTotal      int64            `json:"bytes_written_total"`
	FirstByteMSBuckets     map[string]int64 `json:"first_byte_ms_buckets"`
	ReadWaitMSBuckets      map[string]int64 `json:"read_wait_ms_buckets"`
}

func SnapshotStreamHealth() streamHealthSnapshot {
	return streamHealthSnapshot{
		RequestsTotal:          streamHealth.requestsTotal.Load(),
		SlowFirstByteTotal:     streamHealth.slowFirstByteTotal.Load(),
		ZeroByteResponsesTotal: streamHealth.zeroByteResponsesTotal.Load(),
		ClientDisconnectsTotal: streamHealth.clientDisconnectsTotal.Load(),
		StallsTotal:            streamHealth.stallsTotal.Load(),
		BytesWrittenTotal:      streamHealth.bytesWrittenTotal.Load(),
		FirstByteMSBuckets:     snapshotBuckets(&streamHealth.firstByteBuckets),
		ReadWaitMSBuckets:      snapshotBuckets(&streamHealth.readWaitBuckets),
	}
}

func recordStreamCompleted(firstByte time.Duration, bytesWritten int64, err error) {
	streamHealth.requestsTotal.Add(1)
	streamHealth.bytesWrittenTotal.Add(bytesWritten)

	if bytesWritten == 0 {
		streamHealth.zeroByteResponsesTotal.Add(1)
	}

	if firstByte > 0 {
		recordDurationBucket(&streamHealth.firstByteBuckets, firstByte)
		if firstByte >= streamSlowFirstByteThreshold {
			streamHealth.slowFirstByteTotal.Add(1)
			streamHealth.stallsTotal.Add(1)
		}
	}

	if isClientDisconnect(err) {
		streamHealth.clientDisconnectsTotal.Add(1)
	}
}

func recordStreamReadWait(wait time.Duration) {
	if wait < streamReadWaitThreshold {
		return
	}

	streamHealth.stallsTotal.Add(1)
	recordDurationBucket(&streamHealth.readWaitBuckets, wait)
}

func recordDurationBucket(buckets *[6]atomic.Int64, duration time.Duration) {
	buckets[durationBucketIndex(duration)].Add(1)
}

func snapshotBuckets(buckets *[6]atomic.Int64) map[string]int64 {
	return map[string]int64{
		"le_100":   buckets[0].Load(),
		"le_500":   buckets[1].Load(),
		"le_1000":  buckets[2].Load(),
		"le_3000":  buckets[3].Load(),
		"le_10000": buckets[4].Load(),
		"gt_10000": buckets[5].Load(),
	}
}

func durationBucketIndex(duration time.Duration) int {
	switch {
	case duration <= 100*time.Millisecond:
		return 0
	case duration <= 500*time.Millisecond:
		return 1
	case duration <= time.Second:
		return 2
	case duration <= 3*time.Second:
		return 3
	case duration <= 10*time.Second:
		return 4
	default:
		return 5
	}
}

func isClientDisconnect(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, net.ErrClosed) ||
		errors.Is(err, os.ErrClosed) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNABORTED) {
		return true
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "client disconnected")
}

func resetStreamHealthForTest() {
	streamHealth.requestsTotal.Store(0)
	streamHealth.slowFirstByteTotal.Store(0)
	streamHealth.zeroByteResponsesTotal.Store(0)
	streamHealth.clientDisconnectsTotal.Store(0)
	streamHealth.stallsTotal.Store(0)
	streamHealth.bytesWrittenTotal.Store(0)

	for index := range streamHealth.firstByteBuckets {
		streamHealth.firstByteBuckets[index].Store(0)
		streamHealth.readWaitBuckets[index].Store(0)
	}
}
