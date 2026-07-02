package settings

import "strings"

const (
	StartupPreloadPolicySkipActive = "skip-active"
	StartupPreloadPolicyLegacy     = "legacy"
)

func NormalizeStartupPreloadPolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "", StartupPreloadPolicySkipActive:
		return StartupPreloadPolicySkipActive
	case StartupPreloadPolicyLegacy:
		return StartupPreloadPolicyLegacy
	default:
		return StartupPreloadPolicySkipActive
	}
}
