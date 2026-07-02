package settings

import "testing"

func TestNormalizeCoreProfile(t *testing.T) {
	cases := map[string]string{
		"":                  "custom",
		"custom":            "custom",
		"LOW-END":           "low-end",
		"LOW-CPU":           "low-cpu",
		"balanced":          "balanced",
		"TCP-ONLY-BALANCED": "tcp-only-balanced",
		"high-throughput":   "high-throughput",
		"nas":               "nas",
		"unknown":           "custom",
	}
	for in, want := range cases {
		if got := normalizeCoreProfile(in); got != want {
			t.Fatalf("normalizeCoreProfile(%q)=%q want=%q", in, got, want)
		}
	}
}

func TestLowCPUProfileDefaults(t *testing.T) {
	sets := &BTSets{}
	applyCoreProfilePreset(sets, "low-cpu")

	if !sets.DisableUTP {
		t.Fatal("low-cpu profile must disable uTP")
	}

	if sets.ConnectionsLimit != 12 {
		t.Fatalf("low-cpu ConnectionsLimit = %d, want 12", sets.ConnectionsLimit)
	}

	if sets.CacheSize != 64*1024*1024 {
		t.Fatalf("low-cpu CacheSize = %d, want 64MiB", sets.CacheSize)
	}

	if sets.MaxConcurrentStreams != 2 {
		t.Fatalf("low-cpu MaxConcurrentStreams = %d, want 2", sets.MaxConcurrentStreams)
	}

	if sets.MaxUniquePlaybackTorrents != 2 {
		t.Fatalf("low-cpu MaxUniquePlaybackTorrents = %d, want 2", sets.MaxUniquePlaybackTorrents)
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

func TestApplyCoreProfileMaterializesPreset(t *testing.T) {
	sets := &BTSets{
		CoreProfile:      "TCP-ONLY-BALANCED",
		ConnectionsLimit: 33,
	}

	ApplyCoreProfile(sets)

	if sets.CoreProfile != "tcp-only-balanced" {
		t.Fatalf("CoreProfile = %q, want tcp-only-balanced", sets.CoreProfile)
	}

	if !sets.DisableUTP {
		t.Fatal("tcp-only-balanced profile must disable uTP")
	}

	if sets.ConnectionsLimit != 33 {
		t.Fatalf("ConnectionsLimit override = %d, want 33", sets.ConnectionsLimit)
	}

	if sets.CacheSize != 64*1024*1024 {
		t.Fatalf("CacheSize = %d, want 64MiB from profile", sets.CacheSize)
	}

	if sets.MaxUniquePlaybackTorrents != 2 {
		t.Fatalf("MaxUniquePlaybackTorrents = %d, want 2", sets.MaxUniquePlaybackTorrents)
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

	if sets.MaxUniquePlaybackTorrents != 0 {
		t.Fatalf("balanced MaxUniquePlaybackTorrents = %d, want compatibility default 0", sets.MaxUniquePlaybackTorrents)
	}

	if sets.DiskSyncPolicy != "periodic" {
		t.Fatalf("unexpected balanced DiskSyncPolicy: %s", sets.DiskSyncPolicy)
	}
}

func TestTCPOnlyBalancedProfileDefaults(t *testing.T) {
	sets := &BTSets{}
	applyCoreProfilePreset(sets, "tcp-only-balanced")

	if !sets.DisableUTP {
		t.Fatal("tcp-only-balanced profile must disable uTP")
	}

	if sets.DisableTCP {
		t.Fatal("tcp-only-balanced profile must keep TCP enabled")
	}

	if sets.DisableDHT {
		t.Fatal("tcp-only-balanced profile must keep DHT enabled")
	}

	if sets.DisablePEX {
		t.Fatal("tcp-only-balanced profile must keep PEX enabled")
	}

	if sets.ConnectionsLimit != 25 {
		t.Fatalf("tcp-only-balanced ConnectionsLimit = %d, want 25", sets.ConnectionsLimit)
	}

	if sets.CacheSize != 64*1024*1024 {
		t.Fatalf("tcp-only-balanced CacheSize = %d, want 64MiB", sets.CacheSize)
	}

	if sets.StreamQueueWaitSec != 3 {
		t.Fatalf("tcp-only-balanced StreamQueueWaitSec = %d, want 3", sets.StreamQueueWaitSec)
	}

	if sets.MaxUniquePlaybackTorrents != 2 {
		t.Fatalf("tcp-only-balanced MaxUniquePlaybackTorrents = %d, want 2", sets.MaxUniquePlaybackTorrents)
	}
}
