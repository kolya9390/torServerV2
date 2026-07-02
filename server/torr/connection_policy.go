package torr

import (
	"strings"
	"time"

	"server/settings"
)

const (
	defaultEstablishedConns            = 50
	lowCPUCoreProfile                  = "low-cpu"
	tcpOnlyBalancedCoreProfile         = "tcp-only-balanced"
	tcpOnlyBalancedPeerReliefMinAge    = time.Minute
	maxDebugTotalHalfOpenConnsOverride = 2000
	maxDebugTrackerBudgetOverride      = 256
)

type connectionPolicy struct {
	configuredLimit       int
	effectiveConns        int
	peerLowWater          int
	peerHighWater         int
	totalHalfOpen         int
	trackerBudget         int
	lowCPU                bool
	debugOverride         int
	debugHalfOpenOverride int
	debugTrackerOverride  int
	debugStablePeerCap    int
}

// ConnectionPolicySnapshot is a typed diagnostic view of the torrent connection policy.
type ConnectionPolicySnapshot struct {
	ConnectionsLimit      int
	EffectiveConns        int
	PeerLowWater          int
	PeerHighWater         int
	TotalHalfOpen         int
	TrackerBudget         int
	LowCPUProfile         bool
	DebugOverride         int
	DebugHalfOpenOverride int
	DebugTrackerOverride  int
	DebugStablePeerCap    int
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
	debugCfg := sets.DebugConfig()
	debugOverride := debugEstablishedConnsOverride(debugCfg)
	debugStablePeerCap := debugStablePeerCap(debugCfg, effectiveConns)

	if debugOverride > 0 {
		effectiveConns = debugOverride
	}

	lowWater, highWater := peerWatermarksForProfile(effectiveConns, lowCPU)
	totalHalfOpen := totalHalfOpenConnsForPolicy(effectiveConns)
	debugHalfOpenOverride := debugTotalHalfOpenConnsOverride(debugCfg)

	if debugHalfOpenOverride > 0 {
		totalHalfOpen = maxInt(debugHalfOpenOverride, effectiveConns)
	}

	trackerBudget := trackerBudgetForSettings(sets, lowCPU)
	debugTrackerOverride := debugTrackerBudgetOverride(debugCfg)

	if debugTrackerOverride > 0 {
		trackerBudget = debugTrackerOverride
	}

	return connectionPolicy{
		configuredLimit:       configuredLimit,
		effectiveConns:        effectiveConns,
		peerLowWater:          lowWater,
		peerHighWater:         highWater,
		totalHalfOpen:         totalHalfOpen,
		trackerBudget:         trackerBudget,
		lowCPU:                lowCPU,
		debugOverride:         debugOverride,
		debugHalfOpenOverride: debugHalfOpenOverride,
		debugTrackerOverride:  debugTrackerOverride,
		debugStablePeerCap:    debugStablePeerCap,
	}
}

func debugEstablishedConnsOverride(debugCfg settings.DebugConfig) int {
	if !debugCfg.EnableDebug {
		return 0
	}

	return maxInt(debugCfg.EstablishedConnsOverride, 0)
}

func debugTotalHalfOpenConnsOverride(debugCfg settings.DebugConfig) int {
	if !debugCfg.EnableDebug {
		return 0
	}

	return boundedPositiveInt(debugCfg.TotalHalfOpenConnsOverride, maxDebugTotalHalfOpenConnsOverride)
}

func debugTrackerBudgetOverride(debugCfg settings.DebugConfig) int {
	if !debugCfg.EnableDebug {
		return 0
	}

	return boundedPositiveInt(debugCfg.TrackerBudgetOverride, maxDebugTrackerBudgetOverride)
}

func debugStablePeerCap(debugCfg settings.DebugConfig, effectiveConns int) int {
	return stablePeerCapForDebug(debugCfg, effectiveConns)
}

func boundedPositiveInt(value, maxValue int) int {
	if value <= 0 {
		return 0
	}

	if maxValue > 0 && value > maxValue {
		return maxValue
	}

	return value
}

func isLowCPUCoreProfile(profile string) bool {
	return strings.EqualFold(strings.TrimSpace(profile), lowCPUCoreProfile)
}

func isTCPOnlyBalancedCoreProfile(profile string) bool {
	return strings.EqualFold(strings.TrimSpace(profile), tcpOnlyBalancedCoreProfile)
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

func totalHalfOpenConnsForPolicy(effectiveConns int) int {
	return maxInt(effectiveConns*8, 200)
}

// EffectiveEstablishedConnsForLimit exposes the configured connection-budget policy for diagnostics.
func EffectiveEstablishedConnsForLimit(userLimit int) int {
	return effectiveEstablishedConns(userLimit, defaultEstablishedConns)
}

// ConnectionPolicyForSettings exposes the measured connection policy for diagnostics and tests.
func ConnectionPolicyForSettings(sets *settings.BTSets) ConnectionPolicySnapshot {
	policy := connectionPolicyForSettings(sets, defaultEstablishedConns)

	return ConnectionPolicySnapshot{
		ConnectionsLimit:      policy.configuredLimit,
		EffectiveConns:        policy.effectiveConns,
		PeerLowWater:          policy.peerLowWater,
		PeerHighWater:         policy.peerHighWater,
		TotalHalfOpen:         policy.totalHalfOpen,
		TrackerBudget:         policy.trackerBudget,
		LowCPUProfile:         policy.lowCPU,
		DebugOverride:         policy.debugOverride,
		DebugHalfOpenOverride: policy.debugHalfOpenOverride,
		DebugTrackerOverride:  policy.debugTrackerOverride,
		DebugStablePeerCap:    policy.debugStablePeerCap,
	}
}
