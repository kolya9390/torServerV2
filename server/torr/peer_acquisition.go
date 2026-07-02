package torr

import (
	"sync/atomic"
	"time"

	anacrolix "github.com/anacrolix/torrent"

	"server/settings"
)

const (
	peerAcquisitionBoostMinActiveTorrents = 2
	peerAcquisitionBoostCooldown          = 45 * time.Second
	peerAcquisitionBoostDuration          = 10 * time.Second
	peerAcquisitionBoostMaxDHTServers     = 1
	peerAcquisitionWeakActivePeersFloor   = 8
	peerAcquisitionWeakSeedersFloor       = 8
)

type peerAcquisitionBoostInput struct {
	activePlaybackTorrents int
	activeReaders          int
	activePeers            int
	connectedSeeders       int
}

type dhtPeerBoostAnnounce struct {
	done <-chan struct{}
	stop func()
}

type peerAcquisitionBoostCounters struct {
	eligibleTotal        atomic.Int64
	startedTotal         atomic.Int64
	cooldownSkippedTotal atomic.Int64
	noDHTServersTotal    atomic.Int64
	announceErrorsTotal  atomic.Int64
	notWeakSkippedTotal  atomic.Int64
	completedTotal       atomic.Int64
	stoppedTotal         atomic.Int64
	activeBoosts         atomic.Int64
}

type peerAcquisitionBoostSnapshot struct {
	EligibleTotal        int64 `json:"eligible_total"`
	StartedTotal         int64 `json:"started_total"`
	CooldownSkippedTotal int64 `json:"cooldown_skipped_total"`
	NoDHTServersTotal    int64 `json:"no_dht_servers_total"`
	AnnounceErrorsTotal  int64 `json:"announce_errors_total"`
	NotWeakSkippedTotal  int64 `json:"not_weak_skipped_total"`
	CompletedTotal       int64 `json:"completed_total"`
	StoppedTotal         int64 `json:"stopped_total"`
	ActiveBoosts         int64 `json:"active_boosts"`
}

var peerAcquisitionBoost peerAcquisitionBoostCounters

func (t *Torrent) maybeBoostPeerAcquisition(activePlaybackTorrents int) {
	if t == nil || t.Torrent == nil || t.bt == nil || t.bt.client == nil {
		return
	}

	sets := t.currentSettings()
	input, ok := t.peerAcquisitionBoostInput(activePlaybackTorrents)
	if !ok {
		return
	}

	decision := shouldBoostPeerAcquisition(sets, input)
	if !decision.enabled {
		if decision.notWeak {
			recordPeerBoostNotWeakSkipped(sets.DebugConfig().EnableDebug)
		}

		return
	}

	debugEnabled := sets.DebugConfig().EnableDebug
	recordPeerBoostEligible(debugEnabled)

	servers := t.bt.client.DhtServers()
	if len(servers) == 0 {
		recordPeerBoostNoDHTServers(debugEnabled)

		return
	}

	now := time.Now()
	if !t.claimPeerAcquisitionBoost(now, peerAcquisitionBoostCooldown) {
		recordPeerBoostCooldownSkipped(debugEnabled)

		return
	}

	torrentRef := t.Torrent
	closed := t.lifecycle.closed

	go runDHTPeerAcquisitionBoost(torrentRef, servers, closed, peerAcquisitionBoostDuration, debugEnabled)
}

type peerAcquisitionBoostDecision struct {
	enabled bool
	notWeak bool
}

func (t *Torrent) peerAcquisitionBoostInput(activePlaybackTorrents int) (peerAcquisitionBoostInput, bool) {
	snapshot, ok := t.RuntimeMetricsSnapshot()
	if !ok {
		return peerAcquisitionBoostInput{}, false
	}

	return peerAcquisitionBoostInput{
		activePlaybackTorrents: activePlaybackTorrents,
		activeReaders:          snapshot.ActiveReaders,
		activePeers:            snapshot.ActivePeers,
		connectedSeeders:       snapshot.ConnectedSeeders,
	}, true
}

func shouldBoostPeerAcquisition(
	sets *settings.BTSets,
	input peerAcquisitionBoostInput,
) peerAcquisitionBoostDecision {
	if sets == nil {
		return peerAcquisitionBoostDecision{}
	}

	if input.activePlaybackTorrents < peerAcquisitionBoostMinActiveTorrents {
		return peerAcquisitionBoostDecision{}
	}

	if sets.NetworkConfig().DisableDHT {
		return peerAcquisitionBoostDecision{}
	}

	if !isTCPOnlyBalancedCoreProfile(sets.CoreProfile) {
		return peerAcquisitionBoostDecision{}
	}

	if input.activeReaders <= 0 {
		return peerAcquisitionBoostDecision{}
	}

	if !isWeakPeerAcquisitionTorrent(input) {
		return peerAcquisitionBoostDecision{notWeak: true}
	}

	return peerAcquisitionBoostDecision{enabled: true}
}

func isWeakPeerAcquisitionTorrent(input peerAcquisitionBoostInput) bool {
	return input.activePeers < peerAcquisitionWeakActivePeersFloor ||
		input.connectedSeeders < peerAcquisitionWeakSeedersFloor
}

func (t *Torrent) claimPeerAcquisitionBoost(now time.Time, cooldown time.Duration) bool {
	if t == nil {
		return false
	}

	next := now.UnixNano()

	for {
		cur := t.lifecycle.lastPeerBoostNano.Load()
		if cur > 0 && now.Sub(time.Unix(0, cur)) < cooldown {
			return false
		}

		if t.lifecycle.lastPeerBoostNano.CompareAndSwap(cur, next) {
			return true
		}
	}
}

func runDHTPeerAcquisitionBoost(
	torrentRef *anacrolix.Torrent,
	servers []anacrolix.DhtServer,
	closed <-chan struct{},
	duration time.Duration,
	debugEnabled bool,
) {
	if torrentRef == nil || len(servers) == 0 || duration <= 0 {
		return
	}

	announces := startDHTPeerBoostAnnounces(torrentRef, servers, debugEnabled)
	if len(announces) == 0 {
		return
	}

	recordPeerBoostStarted(debugEnabled)
	defer recordPeerBoostCompleted(debugEnabled)

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-closed:
		recordPeerBoostStopped(debugEnabled)
	case <-timer.C:
	}

	for _, announce := range announces {
		if announce.stop != nil {
			announce.stop()
		}
	}

	waitForDHTPeerBoostAnnounces(announces)
}

func startDHTPeerBoostAnnounces(
	torrentRef *anacrolix.Torrent,
	servers []anacrolix.DhtServer,
	debugEnabled bool,
) []dhtPeerBoostAnnounce {
	if len(servers) > peerAcquisitionBoostMaxDHTServers {
		servers = servers[:peerAcquisitionBoostMaxDHTServers]
	}

	announces := make([]dhtPeerBoostAnnounce, 0, len(servers))

	for _, server := range servers {
		if server == nil {
			continue
		}

		done, stop, err := torrentRef.AnnounceToDht(server)
		if err != nil {
			recordPeerBoostAnnounceError(debugEnabled)

			if stop != nil {
				stop()
			}

			continue
		}

		announces = append(announces, dhtPeerBoostAnnounce{
			done: done,
			stop: stop,
		})
	}

	return announces
}

// SnapshotPeerAcquisitionBoost returns debug-only peer acquisition boost counters.
func SnapshotPeerAcquisitionBoost() peerAcquisitionBoostSnapshot {
	return peerAcquisitionBoostSnapshot{
		EligibleTotal:        peerAcquisitionBoost.eligibleTotal.Load(),
		StartedTotal:         peerAcquisitionBoost.startedTotal.Load(),
		CooldownSkippedTotal: peerAcquisitionBoost.cooldownSkippedTotal.Load(),
		NoDHTServersTotal:    peerAcquisitionBoost.noDHTServersTotal.Load(),
		AnnounceErrorsTotal:  peerAcquisitionBoost.announceErrorsTotal.Load(),
		NotWeakSkippedTotal:  peerAcquisitionBoost.notWeakSkippedTotal.Load(),
		CompletedTotal:       peerAcquisitionBoost.completedTotal.Load(),
		StoppedTotal:         peerAcquisitionBoost.stoppedTotal.Load(),
		ActiveBoosts:         peerAcquisitionBoost.activeBoosts.Load(),
	}
}

func recordPeerBoostEligible(debugEnabled bool) {
	if debugEnabled {
		peerAcquisitionBoost.eligibleTotal.Add(1)
	}
}

func recordPeerBoostStarted(debugEnabled bool) {
	if debugEnabled {
		peerAcquisitionBoost.startedTotal.Add(1)
		peerAcquisitionBoost.activeBoosts.Add(1)
	}
}

func recordPeerBoostCooldownSkipped(debugEnabled bool) {
	if debugEnabled {
		peerAcquisitionBoost.cooldownSkippedTotal.Add(1)
	}
}

func recordPeerBoostNoDHTServers(debugEnabled bool) {
	if debugEnabled {
		peerAcquisitionBoost.noDHTServersTotal.Add(1)
	}
}

func recordPeerBoostAnnounceError(debugEnabled bool) {
	if debugEnabled {
		peerAcquisitionBoost.announceErrorsTotal.Add(1)
	}
}

func recordPeerBoostNotWeakSkipped(debugEnabled bool) {
	if debugEnabled {
		peerAcquisitionBoost.notWeakSkippedTotal.Add(1)
	}
}

func recordPeerBoostCompleted(debugEnabled bool) {
	if debugEnabled {
		peerAcquisitionBoost.completedTotal.Add(1)
		peerAcquisitionBoost.activeBoosts.Add(-1)
	}
}

func recordPeerBoostStopped(debugEnabled bool) {
	if debugEnabled {
		peerAcquisitionBoost.stoppedTotal.Add(1)
	}
}

func resetPeerAcquisitionBoostForTest() {
	peerAcquisitionBoost.eligibleTotal.Store(0)
	peerAcquisitionBoost.startedTotal.Store(0)
	peerAcquisitionBoost.cooldownSkippedTotal.Store(0)
	peerAcquisitionBoost.noDHTServersTotal.Store(0)
	peerAcquisitionBoost.announceErrorsTotal.Store(0)
	peerAcquisitionBoost.notWeakSkippedTotal.Store(0)
	peerAcquisitionBoost.completedTotal.Store(0)
	peerAcquisitionBoost.stoppedTotal.Store(0)
	peerAcquisitionBoost.activeBoosts.Store(0)
}

func waitForDHTPeerBoostAnnounces(announces []dhtPeerBoostAnnounce) {
	const stopWait = 500 * time.Millisecond

	for _, announce := range announces {
		if announce.done == nil {
			continue
		}

		timer := time.NewTimer(stopWait)
		select {
		case <-announce.done:
		case <-timer.C:
		}
		timer.Stop()
	}
}
