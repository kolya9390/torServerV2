package torr

import (
	"testing"
	"time"

	"server/settings"
	"server/torr/state"
)

func TestEstimatePlaybackTorrents(t *testing.T) {
	tests := []struct {
		name          string
		activeStreams int32
		localReaders  int
		want          int
	}{
		{name: "no streams", activeStreams: 0, localReaders: 0, want: 1},
		{name: "single stream", activeStreams: 1, localReaders: 1, want: 1},
		{name: "two streams same torrent", activeStreams: 2, localReaders: 2, want: 1},
		{name: "two streams different torrents", activeStreams: 2, localReaders: 1, want: 2},
		{name: "local readers higher than streams", activeStreams: 2, localReaders: 5, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimatePlaybackTorrents(tt.activeStreams, tt.localReaders)
			if got != tt.want {
				t.Fatalf("estimatePlaybackTorrents(%d, %d) = %d, want %d", tt.activeStreams, tt.localReaders, got, tt.want)
			}
		})
	}
}

func TestAdaptiveCacheCapacity(t *testing.T) {
	tests := []struct {
		name             string
		baseCap          int64
		playbackTorrents int
		want             int64
	}{
		{name: "zero base", baseCap: 0, playbackTorrents: 1, want: 0},
		{name: "single playback keeps configured cache", baseCap: 256 << 20, playbackTorrents: 1, want: 256 << 20},
		{name: "two playback keeps configured cache", baseCap: 256 << 20, playbackTorrents: 2, want: 256 << 20},
		{name: "tiny cache stays configured", baseCap: 64 << 20, playbackTorrents: 1, want: 64 << 20},
		{name: "two playback does not expand cache", baseCap: 64 << 20, playbackTorrents: 2, want: 64 << 20},
		{name: "medium cache stays configured", baseCap: 128 << 20, playbackTorrents: 2, want: 128 << 20},
		{name: "many playback still configured", baseCap: 256 << 20, playbackTorrents: 10, want: 256 << 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adaptiveCacheCapacity(tt.baseCap, tt.playbackTorrents)
			if got != tt.want {
				t.Fatalf("adaptiveCacheCapacity(%d, %d) = %d, want %d", tt.baseCap, tt.playbackTorrents, got, tt.want)
			}
		})
	}
}

func TestAdaptiveReadahead(t *testing.T) {
	tests := []struct {
		name             string
		cacheCap         int64
		playbackTorrents int
		cfg              settings.StreamConfig
		want             int64
	}{
		{name: "default single stream uses configured default max", cacheCap: 256 << 20, playbackTorrents: 1, want: 64 << 20},
		{name: "default two streams keep configured max", cacheCap: 256 << 20, playbackTorrents: 2, want: 64 << 20},
		{name: "high throughput honors larger max", cacheCap: 256 << 20, playbackTorrents: 1, cfg: settings.StreamConfig{
			AdaptiveRAMinMB: 4,
			AdaptiveRAMaxMB: 128,
		}, want: 128 << 20},
		{name: "many playback torrents scale down within bounds", cacheCap: 256 << 20, playbackTorrents: 4, cfg: settings.StreamConfig{
			AdaptiveRAMinMB: 4,
			AdaptiveRAMaxMB: 128,
		}, want: 64 << 20},
		{name: "small cache clamp", cacheCap: 8 << 20, playbackTorrents: 4, cfg: settings.StreamConfig{
			AdaptiveRAMinMB: 4,
			AdaptiveRAMaxMB: 64,
		}, want: 8 << 20},
		{name: "min bound protects tiny scaled target", cacheCap: 256 << 20, playbackTorrents: 16, cfg: settings.StreamConfig{
			AdaptiveRAMinMB: 16,
			AdaptiveRAMaxMB: 64,
		}, want: 16 << 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adaptiveReadahead(tt.cacheCap, tt.playbackTorrents, tt.cfg)
			if got != tt.want {
				t.Fatalf("adaptiveReadahead(%d, %d, %+v) = %d, want %d",
					tt.cacheCap, tt.playbackTorrents, tt.cfg, got, tt.want)
			}
		})
	}
}

func TestAdaptivePriorityInterval(t *testing.T) {
	tests := []struct {
		name             string
		playbackTorrents int
		want             time.Duration
	}{
		{name: "single", playbackTorrents: 1, want: time.Second},
		{name: "dual", playbackTorrents: 2, want: 2 * time.Second},
		{name: "three", playbackTorrents: 3, want: 2 * time.Second},
		{name: "six", playbackTorrents: 6, want: 2 * time.Second},
		{name: "twelve", playbackTorrents: 12, want: 2 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adaptivePriorityInterval(tt.playbackTorrents)
			if got != tt.want {
				t.Fatalf("adaptivePriorityInterval(%d) = %s, want %s", tt.playbackTorrents, got, tt.want)
			}
		})
	}
}

func TestAdaptiveMaxEstablishedConns(t *testing.T) {
	tests := []struct {
		name             string
		sets             *settings.BTSets
		playbackTorrents int
		localReaders     int
		want             int
	}{
		{name: "idle honors configured peer budget", sets: &settings.BTSets{ConnectionsLimit: 25}, playbackTorrents: 1, localReaders: 0, want: 25},
		{name: "single playback honors configured peer budget", sets: &settings.BTSets{ConnectionsLimit: 25}, playbackTorrents: 1, localReaders: 1, want: 25},
		{name: "dual playback honors configured peer budget", sets: &settings.BTSets{ConnectionsLimit: 25}, playbackTorrents: 2, localReaders: 1, want: 25},
		{name: "many playback keeps configured peer budget", sets: &settings.BTSets{ConnectionsLimit: 25}, playbackTorrents: 4, localReaders: 1, want: 25},
		{name: "higher configured limit preserved", sets: &settings.BTSets{ConnectionsLimit: 96}, playbackTorrents: 1, localReaders: 1, want: 96},
		{name: "high configured limit preserved", sets: &settings.BTSets{ConnectionsLimit: 120}, playbackTorrents: 1, localReaders: 1, want: 120},
		{name: "tcp only balanced two playback keeps configured peer reach", sets: &settings.BTSets{CoreProfile: "tcp-only-balanced", ConnectionsLimit: 25, DisableUTP: true}, playbackTorrents: 2, localReaders: 1, want: 25},
		{name: "low cpu profile honors measured low limit", sets: &settings.BTSets{CoreProfile: "low-cpu", ConnectionsLimit: 12}, playbackTorrents: 2, localReaders: 1, want: 12},
		{name: "low cpu profile default stays bounded", sets: &settings.BTSets{CoreProfile: "low-cpu"}, playbackTorrents: 1, localReaders: 1, want: 24},
		{name: "debug override bounds active policy", sets: &settings.BTSets{EnableDebug: true, DebugEstablishedConnsOverride: 36}, playbackTorrents: 2, localReaders: 1, want: 36},
		{name: "debug override ignored outside debug", sets: &settings.BTSets{DebugEstablishedConnsOverride: 24}, playbackTorrents: 2, localReaders: 1, want: 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adaptiveMaxEstablishedConns(tt.sets, tt.playbackTorrents, tt.localReaders)
			if got != tt.want {
				t.Fatalf("adaptiveMaxEstablishedConns(%+v, %d, %d) = %d, want %d",
					tt.sets, tt.playbackTorrents, tt.localReaders, got, tt.want)
			}
		})
	}
}

func TestAdaptiveMaxEstablishedConnsForReaderAge(t *testing.T) {
	tests := []struct {
		name             string
		sets             *settings.BTSets
		playbackTorrents int
		localReaders     int
		oldestReaderAge  time.Duration
		want             int
	}{
		{name: "custom profile keeps full peer reach without debug cap", sets: &settings.BTSets{
			CoreProfile:      "custom",
			ConnectionsLimit: 25,
		}, playbackTorrents: 2, localReaders: 1, oldestReaderAge: 2 * time.Minute, want: 25},
		{name: "custom profile stable playback applies debug bounded relief", sets: &settings.BTSets{
			CoreProfile:        "custom",
			ConnectionsLimit:   100,
			EnableDebug:        true,
			DebugStablePeerCap: 48,
		}, playbackTorrents: 1, localReaders: 1, oldestReaderAge: 2 * time.Minute, want: 48},
		{name: "debug stable cap keeps full peer reach during startup window", sets: &settings.BTSets{
			CoreProfile:        "tcp-only-balanced",
			ConnectionsLimit:   25,
			EnableDebug:        true,
			DebugStablePeerCap: 22,
		}, playbackTorrents: 2, localReaders: 1, oldestReaderAge: 10 * time.Second, want: 25},
		{name: "debug stable cap applies after startup window", sets: &settings.BTSets{
			CoreProfile:        "tcp-only-balanced",
			ConnectionsLimit:   25,
			EnableDebug:        true,
			DebugStablePeerCap: 22,
		}, playbackTorrents: 2, localReaders: 1, oldestReaderAge: 20 * time.Second, want: 22},
		{name: "tcp only balanced single playback applies debug bounded relief", sets: &settings.BTSets{
			CoreProfile:        "tcp-only-balanced",
			ConnectionsLimit:   25,
			EnableDebug:        true,
			DebugStablePeerCap: 22,
		}, playbackTorrents: 1, localReaders: 1, oldestReaderAge: 2 * time.Minute, want: 22},
		{name: "tcp only balanced same torrent readers keep full peer reach", sets: &settings.BTSets{
			CoreProfile:      "tcp-only-balanced",
			ConnectionsLimit: 25,
		}, playbackTorrents: 1, localReaders: 2, oldestReaderAge: 2 * time.Minute, want: 25},
		{name: "tcp only balanced stable two playback keeps full peer reach without debug cap", sets: &settings.BTSets{
			CoreProfile:      "tcp-only-balanced",
			ConnectionsLimit: 25,
		}, playbackTorrents: 2, localReaders: 1, oldestReaderAge: 2 * time.Minute, want: 25},
		{name: "tcp only balanced stable two playback applies debug bounded relief", sets: &settings.BTSets{
			CoreProfile:        "tcp-only-balanced",
			ConnectionsLimit:   25,
			EnableDebug:        true,
			DebugStablePeerCap: 22,
		}, playbackTorrents: 2, localReaders: 1, oldestReaderAge: 2 * time.Minute, want: 22},
		{name: "debug stable cap ignored outside debug", sets: &settings.BTSets{
			CoreProfile:        "tcp-only-balanced",
			ConnectionsLimit:   25,
			DebugStablePeerCap: 22,
		}, playbackTorrents: 2, localReaders: 1, oldestReaderAge: 2 * time.Minute, want: 25},
		{name: "tcp only balanced lower configured cap is preserved", sets: &settings.BTSets{
			CoreProfile:        "tcp-only-balanced",
			ConnectionsLimit:   16,
			EnableDebug:        true,
			DebugStablePeerCap: 22,
		}, playbackTorrents: 2, localReaders: 1, oldestReaderAge: 2 * time.Minute, want: 16},
		{name: "debug stable cap can bound debug override after warmup", sets: &settings.BTSets{
			CoreProfile:                   "tcp-only-balanced",
			ConnectionsLimit:              25,
			EnableDebug:                   true,
			DebugEstablishedConnsOverride: 30,
			DebugStablePeerCap:            22,
		}, playbackTorrents: 2, localReaders: 1, oldestReaderAge: 2 * time.Minute, want: 22},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adaptiveMaxEstablishedConnsForReaderAge(
				tt.sets,
				tt.playbackTorrents,
				tt.localReaders,
				tt.oldestReaderAge,
			)
			if got != tt.want {
				t.Fatalf("adaptiveMaxEstablishedConnsForReaderAge(%+v, %d, %d, %s) = %d, want %d",
					tt.sets, tt.playbackTorrents, tt.localReaders, tt.oldestReaderAge, got, tt.want)
			}
		})
	}
}

func TestShouldExpireTorrent(t *testing.T) {
	now := time.Now().UnixNano()
	expired := now - int64(time.Second)
	future := now + int64(time.Second)

	tests := []struct {
		name            string
		readers         int
		hasActiveStream bool
		expNs           int64
		stat            state.TorrentStat
		want            bool
	}{
		{name: "active reader", readers: 1, expNs: expired, stat: state.TorrentWorking, want: false},
		{
			name:            "active stream admission",
			hasActiveStream: true,
			expNs:           expired,
			stat:            state.TorrentWorking,
			want:            false,
		},
		{name: "not yet expired", expNs: future, stat: state.TorrentWorking, want: false},
		{name: "wrong state", expNs: expired, stat: state.TorrentPreload, want: false},
		{name: "expired working torrent", expNs: expired, stat: state.TorrentWorking, want: true},
		{name: "expired closed torrent", expNs: expired, stat: state.TorrentClosed, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldExpireTorrent(tt.readers, tt.hasActiveStream, tt.expNs, now, tt.stat)
			if got != tt.want {
				t.Fatalf("shouldExpireTorrent(%d, %v, %d, %v) = %v, want %v",
					tt.readers, tt.hasActiveStream, tt.expNs, tt.stat, got, tt.want)
			}
		})
	}
}

func TestShortenExpiredTime(t *testing.T) {
	torr := &Torrent{}

	torr.AddExpiredTime(time.Minute)
	initial := torr.lifecycle.expiredUnixNano.Load()
	torr.ShortenExpiredTime(5 * time.Second)
	shortened := torr.lifecycle.expiredUnixNano.Load()

	if shortened <= 0 {
		t.Fatal("ShortenExpiredTime() did not set expiration")
	}

	if shortened >= initial {
		t.Fatalf("ShortenExpiredTime() = %d, want earlier than %d", shortened, initial)
	}

	torr.ShortenExpiredTime(time.Minute)

	if got := torr.lifecycle.expiredUnixNano.Load(); got != shortened {
		t.Fatalf("ShortenExpiredTime() moved expiration later: got %d, want %d", got, shortened)
	}

	torr.lifecycle.expiredUnixNano.Store(time.Now().Add(-time.Second).UnixNano())
	torr.ShortenExpiredTime(5 * time.Second)

	grace := torr.lifecycle.expiredUnixNano.Load()
	if grace <= time.Now().UnixNano() {
		t.Fatalf("ShortenExpiredTime() with stale expiration = %d, want future grace period", grace)
	}
}

func TestTouchPlaybackIntentExtendsExpiration(t *testing.T) {
	torr := &Torrent{}
	torr.lifecycle.expiredUnixNano.Store(time.Now().Add(-time.Second).UnixNano())

	torr.TouchPlaybackIntent()

	if got := torr.lifecycle.expiredUnixNano.Load(); got <= time.Now().UnixNano() {
		t.Fatalf("TouchPlaybackIntent() expiration = %d, want future timestamp", got)
	}
}

func TestPostPlaybackDisconnectDelay(t *testing.T) {
	tests := []struct {
		name string
		sets *settings.BTSets
		want time.Duration
	}{
		{name: "nil settings uses default timeout", want: 30 * time.Second},
		{name: "custom profile preserves configured timeout", sets: &settings.BTSets{
			CoreProfile:              "custom",
			TorrentDisconnectTimeout: 30,
		}, want: 30 * time.Second},
		{name: "tcp only balanced caps post playback idle", sets: &settings.BTSets{
			CoreProfile:              "tcp-only-balanced",
			TorrentDisconnectTimeout: 30,
		}, want: 5 * time.Second},
		{name: "tcp only balanced preserves shorter user timeout", sets: &settings.BTSets{
			CoreProfile:              "tcp-only-balanced",
			TorrentDisconnectTimeout: 3,
		}, want: 3 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := postPlaybackDisconnectDelay(tt.sets); got != tt.want {
				t.Fatalf("postPlaybackDisconnectDelay() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestShouldBoostPeerAcquisition(t *testing.T) {
	tests := []struct {
		name                   string
		sets                   *settings.BTSets
		activePlaybackTorrents int
		activeReaders          int
		activePeers            int
		connectedSeeders       int
		want                   bool
		wantNotWeak            bool
	}{
		{name: "nil settings disabled", activePlaybackTorrents: 2, activeReaders: 1, want: false},
		{name: "custom profile disabled", sets: &settings.BTSets{
			CoreProfile: "custom",
		}, activePlaybackTorrents: 2, activeReaders: 1, want: false},
		{name: "single active torrent disabled", sets: &settings.BTSets{
			CoreProfile: "tcp-only-balanced",
		}, activePlaybackTorrents: 1, activeReaders: 1, want: false},
		{name: "dht disabled", sets: &settings.BTSets{
			CoreProfile: "tcp-only-balanced",
			DisableDHT:  true,
		}, activePlaybackTorrents: 2, activeReaders: 1, want: false},
		{name: "inactive reader disabled", sets: &settings.BTSets{
			CoreProfile: "tcp-only-balanced",
		}, activePlaybackTorrents: 2, activeReaders: 0, activePeers: 2, connectedSeeders: 2, want: false},
		{name: "saturated torrent skipped", sets: &settings.BTSets{
			CoreProfile: "tcp-only-balanced",
		}, activePlaybackTorrents: 2, activeReaders: 1, activePeers: 12, connectedSeeders: 10, want: false, wantNotWeak: true},
		{name: "weak active peers enabled", sets: &settings.BTSets{
			CoreProfile: "tcp-only-balanced",
		}, activePlaybackTorrents: 2, activeReaders: 1, activePeers: 6, connectedSeeders: 9, want: true},
		{name: "weak seeders enabled", sets: &settings.BTSets{
			CoreProfile: "tcp-only-balanced",
		}, activePlaybackTorrents: 2, activeReaders: 1, activePeers: 9, connectedSeeders: 6, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldBoostPeerAcquisition(tt.sets, peerAcquisitionBoostInput{
				activePlaybackTorrents: tt.activePlaybackTorrents,
				activeReaders:          tt.activeReaders,
				activePeers:            tt.activePeers,
				connectedSeeders:       tt.connectedSeeders,
			})

			if got.enabled != tt.want {
				t.Fatalf("shouldBoostPeerAcquisition().enabled = %v, want %v", got.enabled, tt.want)
			}

			if got.notWeak != tt.wantNotWeak {
				t.Fatalf("shouldBoostPeerAcquisition().notWeak = %v, want %v", got.notWeak, tt.wantNotWeak)
			}
		})
	}
}

func TestIsWeakPeerAcquisitionTorrent(t *testing.T) {
	tests := []struct {
		name string
		in   peerAcquisitionBoostInput
		want bool
	}{
		{name: "below peer floor", in: peerAcquisitionBoostInput{activePeers: 7, connectedSeeders: 9}, want: true},
		{name: "below seeder floor", in: peerAcquisitionBoostInput{activePeers: 9, connectedSeeders: 7}, want: true},
		{name: "at floors", in: peerAcquisitionBoostInput{
			activePeers:      peerAcquisitionWeakActivePeersFloor,
			connectedSeeders: peerAcquisitionWeakSeedersFloor,
		}, want: false},
		{name: "above floors", in: peerAcquisitionBoostInput{activePeers: 12, connectedSeeders: 10}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isWeakPeerAcquisitionTorrent(tt.in); got != tt.want {
				t.Fatalf("isWeakPeerAcquisitionTorrent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClaimPeerAcquisitionBoost(t *testing.T) {
	torr := &Torrent{}
	now := time.Unix(100, 0)

	if !torr.claimPeerAcquisitionBoost(now, peerAcquisitionBoostCooldown) {
		t.Fatal("first claim should be accepted")
	}

	if torr.claimPeerAcquisitionBoost(now.Add(peerAcquisitionBoostCooldown/2), peerAcquisitionBoostCooldown) {
		t.Fatal("claim inside cooldown should be rejected")
	}

	if !torr.claimPeerAcquisitionBoost(now.Add(peerAcquisitionBoostCooldown), peerAcquisitionBoostCooldown) {
		t.Fatal("claim after cooldown should be accepted")
	}
}

func TestPeerAcquisitionBoostCountersAreDebugOnly(t *testing.T) {
	resetPeerAcquisitionBoostForTest()
	t.Cleanup(resetPeerAcquisitionBoostForTest)

	recordPeerBoostEligible(false)
	recordPeerBoostStarted(false)
	recordPeerBoostCooldownSkipped(false)
	recordPeerBoostNoDHTServers(false)
	recordPeerBoostAnnounceError(false)
	recordPeerBoostNotWeakSkipped(false)
	recordPeerBoostStopped(false)
	recordPeerBoostCompleted(false)

	snapshot := SnapshotPeerAcquisitionBoost()
	if snapshot.EligibleTotal != 0 ||
		snapshot.StartedTotal != 0 ||
		snapshot.CooldownSkippedTotal != 0 ||
		snapshot.NoDHTServersTotal != 0 ||
		snapshot.AnnounceErrorsTotal != 0 ||
		snapshot.NotWeakSkippedTotal != 0 ||
		snapshot.StoppedTotal != 0 ||
		snapshot.CompletedTotal != 0 ||
		snapshot.ActiveBoosts != 0 {
		t.Fatalf("debug-disabled counters changed: %+v", snapshot)
	}

	recordPeerBoostEligible(true)
	recordPeerBoostStarted(true)
	recordPeerBoostCooldownSkipped(true)
	recordPeerBoostNoDHTServers(true)
	recordPeerBoostAnnounceError(true)
	recordPeerBoostNotWeakSkipped(true)
	recordPeerBoostStopped(true)
	recordPeerBoostCompleted(true)

	snapshot = SnapshotPeerAcquisitionBoost()
	if snapshot.EligibleTotal != 1 ||
		snapshot.StartedTotal != 1 ||
		snapshot.CooldownSkippedTotal != 1 ||
		snapshot.NoDHTServersTotal != 1 ||
		snapshot.AnnounceErrorsTotal != 1 ||
		snapshot.NotWeakSkippedTotal != 1 ||
		snapshot.StoppedTotal != 1 ||
		snapshot.CompletedTotal != 1 ||
		snapshot.ActiveBoosts != 0 {
		t.Fatalf("debug-enabled counters = %+v, want one completed boost", snapshot)
	}
}

func TestTrackerBudget(t *testing.T) {
	tests := []struct {
		name       string
		sets       *settings.BTSets
		wantBudget int
	}{
		{name: "default", sets: &settings.BTSets{}, wantBudget: 16},
		{name: "strict network", sets: &settings.BTSets{DisableDHT: true, DisablePEX: true}, wantBudget: 24},
		{name: "low connections", sets: &settings.BTSets{ConnectionsLimit: 12}, wantBudget: 8},
		{name: "high connections", sets: &settings.BTSets{ConnectionsLimit: 100}, wantBudget: 24},
		{name: "tcp only balanced keeps bounded tracker fanout", sets: &settings.BTSets{
			CoreProfile: "tcp-only-balanced", ConnectionsLimit: 25,
		}, wantBudget: 16},
		{name: "tcp only balanced low connections", sets: &settings.BTSets{
			CoreProfile: "tcp-only-balanced", ConnectionsLimit: 12,
		}, wantBudget: 8},
		{name: "debug override", sets: &settings.BTSets{EnableDebug: true, DebugTrackerBudgetOverride: 64}, wantBudget: 64},
		{name: "debug override ignored outside debug", sets: &settings.BTSets{DebugTrackerBudgetOverride: 64}, wantBudget: 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trackerBudget(tt.sets); got != tt.wantBudget {
				t.Fatalf("trackerBudget() = %d, want %d", got, tt.wantBudget)
			}
		})
	}
}
