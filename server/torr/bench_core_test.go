package torr

import (
	"context"
	"sort"
	"testing"
	"time"

	"server/settings"
)

func BenchmarkAdaptiveReadaheadProfile(b *testing.B) {
	in := adaptiveRAInput{
		pieceLength: 1 << 20,
		cacheCap:    256 << 20,
		readers:     2,
		downloadBps: 8 * 1024 * 1024,
		bitrate:     "12000000",
		buffered:    48 << 20,
		currentRA:   16 << 20,
		minRA:       4 << 20,
		maxRA:       64 << 20,
	}

	samples := make([]int64, 0, minInt(b.N, 8192))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		_ = computeAdaptiveReadahead(in)
		if i < cap(samples) {
			samples = append(samples, time.Since(start).Nanoseconds())
		}
	}
	b.StopTimer()
	reportLatencyPercentiles(b, samples)
}

func BenchmarkStreamAdmissionProfile(b *testing.B) {
	resetAdmissionCounters()
	prev := settings.BTsets
	settings.BTsets = &settings.BTSets{
		MaxConcurrentStreams: 1,
		StreamQueueSize:      1,
		StreamQueueWaitSec:   1,
	}
	b.Cleanup(func() {
		settings.BTsets = prev
		resetAdmissionCounters()
	})

	samples := make([]int64, 0, minInt(b.N, 8192))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		release, err := tryAcquireStreamSlot(context.Background())
		if err == nil {
			release()
		}
		if i < cap(samples) {
			samples = append(samples, time.Since(start).Nanoseconds())
		}
	}
	b.StopTimer()
	reportLatencyPercentiles(b, samples)
}

func reportLatencyPercentiles(b *testing.B, samples []int64) {
	if len(samples) == 0 {
		return
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	b.ReportMetric(float64(percentile(samples, 50)), "p50_ns/op")
	b.ReportMetric(float64(percentile(samples, 95)), "p95_ns/op")
	b.ReportMetric(float64(percentile(samples, 99)), "p99_ns/op")
}

func percentile(samples []int64, p int) int64 {
	if len(samples) == 0 {
		return 0
	}
	if p <= 0 {
		return samples[0]
	}
	if p >= 100 {
		return samples[len(samples)-1]
	}
	idx := (len(samples) - 1) * p / 100
	return samples[idx]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
