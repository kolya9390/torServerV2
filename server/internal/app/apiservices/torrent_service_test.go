package apiservices

import (
	"testing"

	sets "server/settings"
	"server/torr"
)

type runtimeSignalsStub struct {
	hasRuntimeBackend      bool
	activePlaybackTorrents int
	activeStreams          int32
}

func (s runtimeSignalsStub) HasRuntimeBackend() bool {
	return s.hasRuntimeBackend
}

func (s runtimeSignalsStub) ActivePlaybackTorrents() int {
	return s.activePlaybackTorrents
}

func (s runtimeSignalsStub) ActiveStreams() int32 {
	return s.activeStreams
}

func TestDecideStartupPreload(t *testing.T) {
	tests := []struct {
		name    string
		policy  string
		signals runtimeSignalsStub
		nilSig  bool
		want    preloadDecision
	}{
		{
			name:   "nil runtime signals allows explicit first preload",
			nilSig: true,
			want:   preloadAllowed,
		},
		{
			name: "runtime backend idle allows explicit first preload",
			signals: runtimeSignalsStub{
				hasRuntimeBackend: true,
			},
			want: preloadAllowed,
		},
		{
			name:   "runtime backend with active playback skips preload",
			policy: sets.StartupPreloadPolicySkipActive,
			signals: runtimeSignalsStub{
				hasRuntimeBackend:      true,
				activePlaybackTorrents: 1,
				activeStreams:          1,
			},
			want: preloadSkippedActivePlayback,
		},
		{
			name:   "legacy policy allows one active playback for rollback",
			policy: sets.StartupPreloadPolicyLegacy,
			signals: runtimeSignalsStub{
				hasRuntimeBackend:      true,
				activePlaybackTorrents: 1,
				activeStreams:          1,
			},
			want: preloadAllowed,
		},
		{
			name:   "legacy policy skips multi playback",
			policy: sets.StartupPreloadPolicyLegacy,
			signals: runtimeSignalsStub{
				hasRuntimeBackend:      true,
				activePlaybackTorrents: 2,
				activeStreams:          2,
			},
			want: preloadSkippedActivePlayback,
		},
		{
			name: "stream counter fallback with active stream skips preload",
			signals: runtimeSignalsStub{
				activeStreams: 1,
			},
			want: preloadSkippedActiveStream,
		},
		{
			name: "stream counter fallback idle allows explicit first preload",
			want: preloadAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var signals torr.RuntimeSignals
			if !tt.nilSig {
				signals = tt.signals
			}

			if got := decideStartupPreload(tt.policy, signals); got != tt.want {
				t.Fatalf("decideStartupPreload() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnqueuePreloadAcceptsIntentionalSkip(t *testing.T) {
	tests := []struct {
		name    string
		signals runtimeSignalsStub
	}{
		{
			name: "active playback skip is accepted as no-op",
			signals: runtimeSignalsStub{
				hasRuntimeBackend:      true,
				activePlaybackTorrents: 1,
				activeStreams:          1,
			},
		},
		{
			name: "active stream fallback skip is accepted as no-op",
			signals: runtimeSignalsStub{
				activeStreams: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := torrentService{runtimeSignals: tt.signals}

			if !svc.EnqueuePreload(wrapTorrent(&torr.Torrent{}), 0) {
				t.Fatal("expected skipped preload to be accepted")
			}
		})
	}
}
