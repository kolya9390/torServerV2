package torr

import (
	"strings"

	"server/settings"
)

const (
	defaultEstablishedConns = 50
	lowCPUCoreProfile       = "low-cpu"
)

type connectionPolicy struct {
	configuredLimit int
	effectiveConns  int
	peerLowWater    int
	peerHighWater   int
	trackerBudget   int
	lowCPU          bool
	debugOverride   int
}

// ConnectionPolicySnapshot is a typed diagnostic view of the torrent connection policy.
type ConnectionPolicySnapshot struct {
	ConnectionsLimit int
	EffectiveConns   int
	PeerLowWater     int
	PeerHighWater    int
	TrackerBudget    int
	LowCPUProfile    bool
	DebugOverride    int
}

func connectionPolicyForSettings(sets *settings.BTSets, defaultConns int) connectionPolicy {
	if sets == nil {
		sets = &settings.BTSets{}
	}

	if defaultConns <= 0 {
		defaultConns = defaultEstablishedConns
	}

	lowCPU := isLowCPUCoreProfile(sets.CoreProfile)
	configuredLimit := sets.NetworkConfig().ConnectionsLimit
	effectiveConns := effectiveEstablishedConnsForProfile(configuredLimit, defaultConns, lowCPU)
	debugOverride := debugEstablishedConnsOverride(sets)

	if debugOverride > 0 {
		effectiveConns = debugOverride
	}

	lowWater, highWater := peerWatermarksForProfile(effectiveConns, lowCPU)

	return connectionPolicy{
		configuredLimit: configuredLimit,
		effectiveConns:  effectiveConns,
		peerLowWater:    lowWater,
		peerHighWater:   highWater,
		trackerBudget:   trackerBudgetForSettings(sets, lowCPU),
		lowCPU:          lowCPU,
		debugOverride:   debugOverride,
	}
}

func debugEstablishedConnsOverride(sets *settings.BTSets) int {
	if sets == nil || !sets.DebugConfig().EnableDebug {
		return 0
	}

	return maxInt(sets.DebugConfig().EstablishedConnsOverride, 0)
}

func isLowCPUCoreProfile(profile string) bool {
	return strings.EqualFold(strings.TrimSpace(profile), lowCPUCoreProfile)
}

func effectiveEstablishedConnsForProfile(userLimit, defaultConns int, lowCPU bool) int {
	if defaultConns <= 0 {
		defaultConns = defaultEstablishedConns
	}

	if !lowCPU {
		return effectiveEstablishedConns(userLimit, defaultConns)
	}

	if userLimit <= 0 {
		return 24
	}

	if userLimit < 12 {
		return 12
	}

	return userLimit
}

func peerWatermarksForProfile(effectiveConns int, lowCPU bool) (int, int) {
	if !lowCPU {
		return peerWatermarks(effectiveConns)
	}

	if effectiveConns <= 0 {
		effectiveConns = 24
	}

	low := maxInt(effectiveConns*2, 24)
	high := maxInt(effectiveConns*6, 72)

	if high < low+24 {
		high = low + 24
	}

	return low, high
}

func trackerBudgetForSettings(sets *settings.BTSets, lowCPU bool) int {
	if sets == nil {
		sets = &settings.BTSets{}
	}

	maxTrackers := 16

	if lowCPU {
		maxTrackers = 8
	}

	if sets.DisableDHT && sets.DisablePEX {
		maxTrackers = 24
	}

	if sets.ConnectionsLimit > 0 {
		switch {
		case sets.ConnectionsLimit >= 80:
			maxTrackers = 24
		case sets.ConnectionsLimit <= 16:
			maxTrackers = 8
		}
	}

	return maxTrackers
}

// EffectiveEstablishedConnsForLimit exposes the configured connection-budget policy for diagnostics.
func EffectiveEstablishedConnsForLimit(userLimit int) int {
	return effectiveEstablishedConns(userLimit, defaultEstablishedConns)
}

// ConnectionPolicyForSettings exposes the measured connection policy for diagnostics and tests.
func ConnectionPolicyForSettings(sets *settings.BTSets) ConnectionPolicySnapshot {
	policy := connectionPolicyForSettings(sets, defaultEstablishedConns)

	return ConnectionPolicySnapshot{
		ConnectionsLimit: policy.configuredLimit,
		EffectiveConns:   policy.effectiveConns,
		PeerLowWater:     policy.peerLowWater,
		PeerHighWater:    policy.peerHighWater,
		TrackerBudget:    policy.trackerBudget,
		LowCPUProfile:    policy.lowCPU,
		DebugOverride:    policy.debugOverride,
	}
}
