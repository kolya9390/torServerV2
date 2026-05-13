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
		want             int64
	}{
		{name: "single stream fixed horizon", cacheCap: 256 << 20, playbackTorrents: 1, want: 16 << 20},
		{name: "two streams fixed horizon", cacheCap: 256 << 20, playbackTorrents: 2, want: 16 << 20},
		{name: "medium cache", cacheCap: 64 << 20, playbackTorrents: 4, want: 16 << 20},
		{name: "small cache clamp", cacheCap: 8 << 20, playbackTorrents: 4, want: 8 << 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adaptiveReadahead(tt.cacheCap, tt.playbackTorrents)
			if got != tt.want {
				t.Fatalf("adaptiveReadahead(%d, %d) = %d, want %d", tt.cacheCap, tt.playbackTorrents, got, tt.want)
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
		{name: "dual", playbackTorrents: 2, want: time.Second},
		{name: "three", playbackTorrents: 3, want: time.Second},
		{name: "six", playbackTorrents: 6, want: time.Second},
		{name: "twelve", playbackTorrents: 12, want: time.Second},
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
		configuredLimit  int
		playbackTorrents int
		localReaders     int
		want             int
	}{
		{name: "idle keeps base default", configuredLimit: 25, playbackTorrents: 1, localReaders: 0, want: 50},
		{name: "single playback keeps base default", configuredLimit: 25, playbackTorrents: 1, localReaders: 1, want: 50},
		{name: "dual playback keeps base default", configuredLimit: 25, playbackTorrents: 2, localReaders: 1, want: 50},
		{name: "many playback keeps default floor", configuredLimit: 25, playbackTorrents: 4, localReaders: 1, want: 50},
		{name: "higher configured limit preserved", configuredLimit: 96, playbackTorrents: 1, localReaders: 1, want: 96},
		{name: "high configured limit preserved", configuredLimit: 120, playbackTorrents: 1, localReaders: 1, want: 120},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adaptiveMaxEstablishedConns(tt.configuredLimit, tt.playbackTorrents, tt.localReaders)
			if got != tt.want {
				t.Fatalf("adaptiveMaxEstablishedConns(%d, %d, %d) = %d, want %d",
					tt.configuredLimit, tt.playbackTorrents, tt.localReaders, got, tt.want)
			}
		})
	}
}

func TestShouldExpireTorrent(t *testing.T) {
	now := time.Now().UnixNano()
	expired := now - int64(time.Second)
	future := now + int64(time.Second)

	tests := []struct {
		name    string
		readers int
		expNs   int64
		stat    state.TorrentStat
		want    bool
	}{
		{name: "active reader", readers: 1, expNs: expired, stat: state.TorrentWorking, want: false},
		{name: "not yet expired", expNs: future, stat: state.TorrentWorking, want: false},
		{name: "wrong state", expNs: expired, stat: state.TorrentPreload, want: false},
		{name: "expired working torrent", expNs: expired, stat: state.TorrentWorking, want: true},
		{name: "expired closed torrent", expNs: expired, stat: state.TorrentClosed, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.readers == 0 && tt.expNs < now && (tt.stat == state.TorrentWorking || tt.stat == state.TorrentClosed)
			if got != tt.want {
				t.Fatalf("expired predicate(%d, %d, %v) = %v, want %v",
					tt.readers, tt.expNs, tt.stat, got, tt.want)
			}
		})
	}
}

func TestTrackerBudget(t *testing.T) {
	tests := []struct {
		name       string
		sets       *settings.BTSets
		wantBudget int
	}{
		{name: "default", sets: &settings.BTSets{}, wantBudget: 128},
		{name: "strict network", sets: &settings.BTSets{DisableDHT: true, DisablePEX: true}, wantBudget: 192},
		{name: "low connections", sets: &settings.BTSets{ConnectionsLimit: 12}, wantBudget: 96},
		{name: "high connections", sets: &settings.BTSets{ConnectionsLimit: 100}, wantBudget: 192},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trackerBudget(tt.sets); got != tt.wantBudget {
				t.Fatalf("trackerBudget() = %d, want %d", got, tt.wantBudget)
			}
		})
	}
}
