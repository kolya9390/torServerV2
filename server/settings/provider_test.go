package settings

import "testing"

func TestStaticToBTSetsPreservesOperationalKnobs(t *testing.T) {
	cfg := StaticConfig{
		EnableLPD:                       true,
		LPDIPv6:                         true,
		StartupPreloadPolicy:            StartupPreloadPolicyLegacy,
		DebugTotalHalfOpenConnsOverride: 500,
		DebugTrackerBudgetOverride:      64,
		DebugStablePeerCap:              22,
		DebugMaxUnverifiedBytesMB:       32,
	}

	sets := staticToBTSets(cfg)
	if sets == nil {
		t.Fatal("staticToBTSets() returned nil")
	}

	if !sets.EnableLPD || !sets.LPDIPv6 {
		t.Fatalf("LPD settings were not preserved: enable=%v ipv6=%v", sets.EnableLPD, sets.LPDIPv6)
	}

	if sets.StartupPreloadPolicy != StartupPreloadPolicyLegacy {
		t.Fatalf("StartupPreloadPolicy = %q, want %q", sets.StartupPreloadPolicy, StartupPreloadPolicyLegacy)
	}

	if sets.DebugTotalHalfOpenConnsOverride != 500 {
		t.Fatalf("DebugTotalHalfOpenConnsOverride = %d, want 500", sets.DebugTotalHalfOpenConnsOverride)
	}

	if sets.DebugTrackerBudgetOverride != 64 {
		t.Fatalf("DebugTrackerBudgetOverride = %d, want 64", sets.DebugTrackerBudgetOverride)
	}

	if sets.DebugStablePeerCap != 22 {
		t.Fatalf("DebugStablePeerCap = %d, want 22", sets.DebugStablePeerCap)
	}

	if sets.DebugMaxUnverifiedBytesMB != 32 {
		t.Fatalf("DebugMaxUnverifiedBytesMB = %d, want 32", sets.DebugMaxUnverifiedBytesMB)
	}
}
