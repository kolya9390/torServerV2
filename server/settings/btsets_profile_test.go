package settings

import "testing"

func TestNormalizeCoreProfile(t *testing.T) {
	cases := map[string]string{
		"":                "custom",
		"custom":          "custom",
		"LOW-END":         "low-end",
		"balanced":        "balanced",
		"high-throughput": "high-throughput",
		"nas":             "nas",
		"unknown":         "custom",
	}
	for in, want := range cases {
		if got := normalizeCoreProfile(in); got != want {
			t.Fatalf("normalizeCoreProfile(%q)=%q want=%q", in, got, want)
		}
	}
}

func TestApplyCoreProfilePresetAndOverride(t *testing.T) {
	sets := &BTSets{
		CoreProfile:        "low-end",
		CacheSize:          96 * 1024 * 1024,
		ConnectionsLimit:   33,
		DiskWriteBatchSize: 11,
	}

	normalized := normalizeCoreProfile(sets.CoreProfile)
	applyCoreProfilePreset(sets, normalized)
	applyCoreProfileOverrides(sets, &BTSets{
		CacheSize:          96 * 1024 * 1024,
		ConnectionsLimit:   33,
		DiskWriteBatchSize: 11,
	})

	if sets.CacheSize != 96*1024*1024 {
		t.Fatalf("override CacheSize not applied, got %d", sets.CacheSize)
	}
	if sets.ConnectionsLimit != 33 {
		t.Fatalf("override ConnectionsLimit not applied, got %d", sets.ConnectionsLimit)
	}
	if sets.DiskWriteBatchSize != 11 {
		t.Fatalf("override DiskWriteBatchSize not applied, got %d", sets.DiskWriteBatchSize)
	}
	if sets.StreamQueueSize <= 0 {
		t.Fatalf("expected low-end profile to set positive StreamQueueSize")
	}
}

func TestBalancedProfileDefaults(t *testing.T) {
	sets := &BTSets{}
	applyCoreProfilePreset(sets, "balanced")
	if sets.CacheSize != 64*1024*1024 {
		t.Fatalf("unexpected balanced CacheSize: %d", sets.CacheSize)
	}
	if sets.StreamQueueWaitSec != 3 {
		t.Fatalf("unexpected balanced StreamQueueWaitSec: %d", sets.StreamQueueWaitSec)
	}
	if sets.DiskSyncPolicy != "periodic" {
		t.Fatalf("unexpected balanced DiskSyncPolicy: %s", sets.DiskSyncPolicy)
	}
}
