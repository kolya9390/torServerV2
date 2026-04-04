package torr

import (
	"testing"

	"server/torr/state"
)

func TestTorrentFSMValidLifecycle(t *testing.T) {
	tor := &Torrent{}
	tor.setInitialState(state.TorrentAdded)

	if !tor.transitionState(state.TorrentGettingInfo, "test") {
		t.Fatalf("expected Added -> GettingInfo to be valid")
	}
	if !tor.transitionState(state.TorrentWorking, "test") {
		t.Fatalf("expected GettingInfo -> Working to be valid")
	}
	if !tor.transitionState(state.TorrentPreload, "test") {
		t.Fatalf("expected Working -> Preload to be valid")
	}
	if !tor.transitionState(state.TorrentWorking, "test") {
		t.Fatalf("expected Preload -> Working to be valid")
	}
	if !tor.transitionState(state.TorrentClosed, "test") {
		t.Fatalf("expected Working -> Closed to be valid")
	}
}

func TestTorrentFSMRejectsInvalidTransition(t *testing.T) {
	tor := &Torrent{}
	tor.setInitialState(state.TorrentAdded)

	if tor.transitionState(state.TorrentPreload, "invalid") {
		t.Fatalf("expected Added -> Preload to be rejected")
	}
	if got := tor.currentState(); got != state.TorrentAdded {
		t.Fatalf("state changed after invalid transition: %v", got)
	}
}

func TestTorrentFSMRecoveryTransition(t *testing.T) {
	tor := &Torrent{}
	tor.setInitialState(state.TorrentWorking)

	if !tor.transitionState(state.TorrentGettingInfo, "peer_loss_reconnect") {
		t.Fatalf("expected Working -> GettingInfo recovery transition")
	}
	if !tor.transitionState(state.TorrentWorking, "reconnected") {
		t.Fatalf("expected GettingInfo -> Working recovery transition")
	}
}
