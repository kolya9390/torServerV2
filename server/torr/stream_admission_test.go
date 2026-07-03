package torr

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"server/settings"
)

func TestStreamAdmissionAllowsSameTorrentReadersWithoutNewUniqueSlot(t *testing.T) {
	resetStreamAdmissionForTest()

	sets := &settings.BTSets{
		MaxConcurrentStreams:      3,
		MaxUniquePlaybackTorrents: 1,
		StreamQueueSize:           1,
		StreamQueueWaitSec:        1,
	}

	releaseFirst, err := tryAcquireStream(context.Background(), sets, "torrent-a", 11, false)
	if err != nil {
		t.Fatalf("first acquire err = %v", err)
	}
	defer releaseFirst()

	releaseSecond, err := tryAcquireStream(context.Background(), sets, "torrent-a", 11, false)
	if err != nil {
		t.Fatalf("same torrent acquire err = %v", err)
	}
	defer releaseSecond()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if _, err = tryAcquireStream(ctx, sets, "torrent-b", 22, false); err == nil {
		t.Fatal("different torrent acquire err = nil, want timeout")
	}

	if got, want := GetActiveStreams(), int32(2); got != want {
		t.Fatalf("ActiveStreams = %d, want %d", got, want)
	}
}

func TestStreamAdmissionReleasesUniqueSlotForDifferentTorrent(t *testing.T) {
	resetStreamAdmissionForTest()

	sets := &settings.BTSets{
		MaxConcurrentStreams:      2,
		MaxUniquePlaybackTorrents: 1,
		StreamQueueSize:           1,
		StreamQueueWaitSec:        1,
	}

	release, err := tryAcquireStream(context.Background(), sets, "torrent-a", 11, false)
	if err != nil {
		t.Fatalf("first acquire err = %v", err)
	}

	release()
	release()

	releaseNext, err := tryAcquireStream(context.Background(), sets, "torrent-b", 22, false)
	if err != nil {
		t.Fatalf("different torrent after release err = %v", err)
	}
	defer releaseNext()

	if got, want := GetActiveStreams(), int32(1); got != want {
		t.Fatalf("ActiveStreams = %d, want %d", got, want)
	}
}

func TestStreamAdmissionQueueFullRejectsPredictably(t *testing.T) {
	resetStreamAdmissionForTest()

	sets := &settings.BTSets{
		MaxConcurrentStreams:      1,
		MaxUniquePlaybackTorrents: 0,
		StreamQueueSize:           1,
		StreamQueueWaitSec:        1,
	}

	release, err := tryAcquireStream(context.Background(), sets, "torrent-a", 11, false)
	if err != nil {
		t.Fatalf("first acquire err = %v", err)
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		waitRelease, waitErr := tryAcquireStream(ctx, sets, "torrent-b", 22, false)
		if waitRelease != nil {
			waitRelease()
		}

		errCh <- waitErr
	}()

	waitForAdmissionWaiters(t, 1)

	_, err = tryAcquireStream(context.Background(), sets, "torrent-c", 33, false)
	if err == nil || err.Error() != "stream queue is full, try again later" {
		t.Fatalf("third acquire err = %v, want queue full", err)
	}

	cancel()

	if err = <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting acquire err = %v, want context.Canceled", err)
	}
}

func TestStreamAdmissionCancellationCleansQueueSlot(t *testing.T) {
	resetStreamAdmissionForTest()

	sets := &settings.BTSets{
		MaxConcurrentStreams:      2,
		MaxUniquePlaybackTorrents: 1,
		StreamQueueSize:           1,
		StreamQueueWaitSec:        1,
	}

	release, err := tryAcquireStream(context.Background(), sets, "torrent-a", 11, false)
	if err != nil {
		t.Fatalf("first acquire err = %v", err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if _, err = tryAcquireStream(ctx, sets, "torrent-b", 22, false); err == nil {
		t.Fatal("waiting acquire err = nil, want timeout")
	}

	if got := streamAdmissionWaitersForTest(); got != 0 {
		t.Fatalf("waiters after timeout = %d, want 0", got)
	}
}

func TestStreamAdmissionUnlimitedUniqueTorrentsWhenDisabled(t *testing.T) {
	resetStreamAdmissionForTest()

	sets := &settings.BTSets{
		MaxConcurrentStreams:      3,
		MaxUniquePlaybackTorrents: 0,
		StreamQueueWaitSec:        1,
	}

	releaseA, err := tryAcquireStream(context.Background(), sets, "torrent-a", 11, false)
	if err != nil {
		t.Fatalf("torrent-a acquire err = %v", err)
	}
	defer releaseA()

	releaseB, err := tryAcquireStream(context.Background(), sets, "torrent-b", 22, false)
	if err != nil {
		t.Fatalf("torrent-b acquire err = %v", err)
	}
	defer releaseB()

	releaseC, err := tryAcquireStream(context.Background(), sets, "torrent-c", 33, false)
	if err != nil {
		t.Fatalf("torrent-c acquire err = %v", err)
	}
	defer releaseC()

	if got, want := GetActiveStreams(), int32(3); got != want {
		t.Fatalf("ActiveStreams = %d, want %d", got, want)
	}
}

func TestStreamAdmissionCheckRejectsThirdUniqueWithoutQueueing(t *testing.T) {
	resetStreamAdmissionForTest()

	sets := &settings.BTSets{
		MaxConcurrentStreams:      4,
		MaxUniquePlaybackTorrents: 2,
		StreamQueueSize:           2,
		StreamQueueWaitSec:        3,
	}

	releaseA, err := tryAcquireStream(context.Background(), sets, "torrent-a", 11, true)
	if err != nil {
		t.Fatalf("torrent-a acquire err = %v", err)
	}
	defer releaseA()

	releaseB, err := tryAcquireStream(context.Background(), sets, "torrent-b", 22, true)
	if err != nil {
		t.Fatalf("torrent-b acquire err = %v", err)
	}
	defer releaseB()

	decision := CheckStreamAdmission(sets, "torrent-c", true)
	if decision.Allowed {
		t.Fatal("CheckStreamAdmission allowed third unique torrent, want rejection")
	}

	if decision.Reason != streamAdmissionReasonMaxUniquePlaybackTorrents {
		t.Fatalf("Reason = %q, want %q", decision.Reason, streamAdmissionReasonMaxUniquePlaybackTorrents)
	}

	if decision.RetryAfter != 3*time.Second {
		t.Fatalf("RetryAfter = %s, want 3s", decision.RetryAfter)
	}

	snapshot := SnapshotStreamAdmission()
	if got, want := snapshot.ActiveStreams, int32(2); got != want {
		t.Fatalf("ActiveStreams = %d, want %d", got, want)
	}

	if snapshot.QueuedRequests != 0 || snapshot.QueuedUniqueTorrentRequests != 0 {
		t.Fatalf("queued requests after check = %+v, want zero queue", snapshot)
	}

	if got, want := snapshot.OverloadRejectionsTotal, int64(1); got != want {
		t.Fatalf("OverloadRejectionsTotal = %d, want %d", got, want)
	}
}

func TestStreamAdmissionCheckAllowsSameTorrent(t *testing.T) {
	resetStreamAdmissionForTest()

	sets := &settings.BTSets{
		MaxConcurrentStreams:      2,
		MaxUniquePlaybackTorrents: 1,
		StreamQueueWaitSec:        1,
	}

	release, err := tryAcquireStream(context.Background(), sets, "torrent-a", 11, true)
	if err != nil {
		t.Fatalf("torrent-a acquire err = %v", err)
	}
	defer release()

	decision := CheckStreamAdmission(sets, "torrent-a", true)
	if !decision.Allowed {
		t.Fatalf("CheckStreamAdmission rejected same torrent: %+v", decision)
	}

	if got, want := SnapshotStreamAdmission().ActiveStreams, int32(1); got != want {
		t.Fatalf("ActiveStreams = %d, want unchanged %d", got, want)
	}
}

func TestStreamAdmissionSnapshotIsPrivacySafe(t *testing.T) {
	resetStreamAdmissionForTest()

	sets := &settings.BTSets{
		MaxConcurrentStreams:      2,
		MaxUniquePlaybackTorrents: 2,
		StreamQueueWaitSec:        1,
	}

	release, err := tryAcquireStream(context.Background(), sets, "abcdef123456", 42, true)
	if err != nil {
		t.Fatalf("acquire err = %v", err)
	}
	defer release()

	snapshot := SnapshotStreamAdmission()
	if got, want := snapshot.ActiveUniquePlaybackTorrents, 1; got != want {
		t.Fatalf("ActiveUniquePlaybackTorrents = %d, want %d", got, want)
	}

	if got, want := len(snapshot.Streams), 1; got != want {
		t.Fatalf("len(Streams) = %d, want %d", got, want)
	}

	if got, want := snapshot.Streams[0].TorrentID, uint64(42); got != want {
		t.Fatalf("TorrentID = %d, want %d", got, want)
	}

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("Marshal() err = %v", err)
	}

	joined := strings.ToLower(string(raw))
	for _, forbidden := range []string{"hash", "magnet", "path", "title", "ip", "remote", "abcdef"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("snapshot contains privacy-sensitive field %q: %s", forbidden, joined)
		}
	}
}

func TestStreamAdmissionDebugCounters(t *testing.T) {
	resetStreamAdmissionForTest()

	sets := &settings.BTSets{
		MaxConcurrentStreams:      2,
		MaxUniquePlaybackTorrents: 1,
		StreamQueueSize:           1,
		StreamQueueWaitSec:        1,
	}

	release, err := tryAcquireStream(context.Background(), sets, "torrent-a", 11, true)
	if err != nil {
		t.Fatalf("first acquire err = %v", err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if _, err = tryAcquireStream(ctx, sets, "torrent-b", 22, true); err == nil {
		t.Fatal("waiting acquire err = nil, want timeout")
	}

	snapshot := SnapshotStreamAdmission()
	if got, want := snapshot.AdmissionWaitsTotal, int64(1); got != want {
		t.Fatalf("AdmissionWaitsTotal = %d, want %d", got, want)
	}

	if got, want := snapshot.UniqueTorrentWaitsTotal, int64(1); got != want {
		t.Fatalf("UniqueTorrentWaitsTotal = %d, want %d", got, want)
	}

	if got := snapshot.AdmissionTimeoutsTotal; got != 0 {
		t.Fatalf("AdmissionTimeoutsTotal = %d, want 0 for context cancellation", got)
	}
}

func TestStreamAdmissionOverloadRejectionCounter(t *testing.T) {
	resetStreamAdmissionForTest()

	sets := &settings.BTSets{
		MaxConcurrentStreams:      1,
		MaxUniquePlaybackTorrents: 0,
		StreamQueueSize:           1,
		StreamQueueWaitSec:        1,
	}

	release, err := tryAcquireStream(context.Background(), sets, "torrent-a", 11, true)
	if err != nil {
		t.Fatalf("first acquire err = %v", err)
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		waitRelease, waitErr := tryAcquireStream(ctx, sets, "torrent-b", 22, true)
		if waitRelease != nil {
			waitRelease()
		}

		errCh <- waitErr
	}()

	waitForAdmissionWaiters(t, 1)

	_, err = tryAcquireStream(context.Background(), sets, "torrent-c", 33, true)
	if err == nil || err.Error() != "stream queue is full, try again later" {
		t.Fatalf("third acquire err = %v, want queue full", err)
	}

	if got, want := SnapshotStreamAdmission().OverloadRejectionsTotal, int64(1); got != want {
		t.Fatalf("OverloadRejectionsTotal = %d, want %d", got, want)
	}

	cancel()

	if err = <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting acquire err = %v, want context.Canceled", err)
	}
}

func TestStreamAdmissionSnapshotTracksSameTorrentExtraStreams(t *testing.T) {
	resetStreamAdmissionForTest()

	sets := &settings.BTSets{
		MaxConcurrentStreams:      4,
		MaxUniquePlaybackTorrents: 2,
		StreamQueueWaitSec:        1,
	}

	releaseA1, err := tryAcquireStream(context.Background(), sets, "torrent-a", 11, true)
	if err != nil {
		t.Fatalf("torrent-a first acquire err = %v", err)
	}
	defer releaseA1()

	releaseA2, err := tryAcquireStream(context.Background(), sets, "torrent-a", 11, true)
	if err != nil {
		t.Fatalf("torrent-a second acquire err = %v", err)
	}
	defer releaseA2()

	releaseB, err := tryAcquireStream(context.Background(), sets, "torrent-b", 22, true)
	if err != nil {
		t.Fatalf("torrent-b acquire err = %v", err)
	}
	defer releaseB()

	snapshot := SnapshotStreamAdmission()
	if got, want := snapshot.ActiveStreams, int32(3); got != want {
		t.Fatalf("ActiveStreams = %d, want %d", got, want)
	}

	if got, want := snapshot.ActiveUniquePlaybackTorrents, 2; got != want {
		t.Fatalf("ActiveUniquePlaybackTorrents = %d, want %d", got, want)
	}

	if got, want := snapshot.ExtraSameTorrentStreams, 1; got != want {
		t.Fatalf("ExtraSameTorrentStreams = %d, want %d", got, want)
	}

	if got, want := snapshot.MaxReadersPerTorrent, 2; got != want {
		t.Fatalf("MaxReadersPerTorrent = %d, want %d", got, want)
	}
}

func TestStreamAdmissionDebugOffSkipsCounters(t *testing.T) {
	resetStreamAdmissionForTest()

	sets := &settings.BTSets{
		MaxConcurrentStreams:      2,
		MaxUniquePlaybackTorrents: 1,
		StreamQueueSize:           1,
		StreamQueueWaitSec:        1,
	}

	release, err := tryAcquireStream(context.Background(), sets, "torrent-a", 11, false)
	if err != nil {
		t.Fatalf("first acquire err = %v", err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if _, err = tryAcquireStream(ctx, sets, "torrent-b", 22, false); err == nil {
		t.Fatal("waiting acquire err = nil, want timeout")
	}

	snapshot := SnapshotStreamAdmission()
	if snapshot.AdmissionWaitsTotal != 0 ||
		snapshot.UniqueTorrentWaitsTotal != 0 ||
		snapshot.AdmissionTimeoutsTotal != 0 ||
		snapshot.OverloadRejectionsTotal != 0 {
		t.Fatalf("debug-off counters = %+v, want zero counters", snapshot)
	}
}

func resetStreamAdmissionForTest() {
	atomic.StoreInt32(&activeStreams, 0)
	atomic.StoreInt64(&lastStreamActivityUnixNano, 0)
	streamAdmissionState = newStreamAdmissionState()
}

func streamAdmissionWaitersForTest() int {
	streamAdmissionState.mu.Lock()
	defer streamAdmissionState.mu.Unlock()

	return streamAdmissionState.waiters
}

func waitForAdmissionWaiters(t *testing.T, want int) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := streamAdmissionWaitersForTest(); got == want {
			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatalf("waiters = %d, want %d", streamAdmissionWaitersForTest(), want)
}
