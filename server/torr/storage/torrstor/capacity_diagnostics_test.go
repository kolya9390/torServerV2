package torrstor

import (
	"encoding/json"
	"strings"
	"testing"

	"server/settings"
)

func TestRequestStrategyCapacityDiagnosticsBoundedAndResized(t *testing.T) {
	cache := newRequestStrategyDiagnosticCache(true)
	cache.registerRequestStrategyCapacityDiagnostics(true)
	t.Cleanup(cache.unregisterRequestStrategyCapacityDiagnostics)

	initial := SnapshotRequestStrategyCapacityDiagnostics()
	item := onlyRequestStrategyCapacityItem(t, initial)

	if initial.Status != "ok" || initial.CappedCaches != 1 || initial.UncappedCaches != 0 {
		t.Fatalf("initial diagnostics = %+v, want one capped cache", initial)
	}

	if got, want := item.BoundedRequestablePieceEstimate, int64(32); got != want {
		t.Fatalf("bounded requestable pieces = %d, want %d", got, want)
	}

	cache.SetCapacity(32 << 20)

	resized := SnapshotRequestStrategyCapacityDiagnostics()
	item = onlyRequestStrategyCapacityItem(t, resized)

	if got, want := item.CapacityBytes, int64(32<<20); got != want {
		t.Fatalf("resized capacity = %d, want %d", got, want)
	}

	if got, want := item.BoundedRequestablePieceEstimate, int64(16); got != want {
		t.Fatalf("resized bounded requestable pieces = %d, want %d", got, want)
	}
}

func TestRequestStrategyCapacityDiagnosticsUncappedIsFault(t *testing.T) {
	cache := newRequestStrategyDiagnosticCache(true)
	cache.registerRequestStrategyCapacityDiagnostics(false)
	t.Cleanup(cache.unregisterRequestStrategyCapacityDiagnostics)

	diagnostics := SnapshotRequestStrategyCapacityDiagnostics()
	item := onlyRequestStrategyCapacityItem(t, diagnostics)

	if diagnostics.Status != "fault_missing_capacity" || diagnostics.UncappedCaches != 1 {
		t.Fatalf("diagnostics = %+v, want missing-capacity fault", diagnostics)
	}

	if item.ContractEnabled || item.CapacityCapped || item.Fault != "missing_capacity_contract" {
		t.Fatalf("uncapped item = %+v, want explicit missing-capacity fault", item)
	}
}

func TestRequestStrategyCapacityDiagnosticsClosedCacheIsRemoved(t *testing.T) {
	cache := newRequestStrategyDiagnosticCache(true)
	cache.registerRequestStrategyCapacityDiagnostics(true)

	if err := cache.Close(); err != nil {
		t.Fatalf("close cache: %v", err)
	}

	diagnostics := SnapshotRequestStrategyCapacityDiagnostics()
	if diagnostics.ActiveCaches != 0 || diagnostics.Status != "inactive" {
		t.Fatalf("diagnostics after close = %+v, want inactive", diagnostics)
	}
}

func TestRequestStrategyCapacityDiagnosticsDisabledOutsideDebug(t *testing.T) {
	cache := newRequestStrategyDiagnosticCache(false)
	cache.registerRequestStrategyCapacityDiagnostics(true)
	t.Cleanup(cache.unregisterRequestStrategyCapacityDiagnostics)

	diagnostics := SnapshotRequestStrategyCapacityDiagnostics()
	if diagnostics.ActiveCaches != 0 || len(diagnostics.Caches) != 0 {
		t.Fatalf("debug-disabled diagnostics = %+v, want no registered cache", diagnostics)
	}
}

func TestRequestStrategyCapacityDiagnosticsBoundCardinalityAndRemainPrivate(t *testing.T) {
	caches := make([]*Cache, 0, maxRequestStrategyCapacityItems+1)

	for range maxRequestStrategyCapacityItems + 1 {
		cache := newRequestStrategyDiagnosticCache(true)
		cache.registerRequestStrategyCapacityDiagnostics(true)
		caches = append(caches, cache)
	}

	t.Cleanup(func() {
		for _, cache := range caches {
			cache.unregisterRequestStrategyCapacityDiagnostics()
		}
	})

	diagnostics := SnapshotRequestStrategyCapacityDiagnostics()
	if got, want := len(diagnostics.Caches), maxRequestStrategyCapacityItems; got != want {
		t.Fatalf("diagnostic items = %d, want %d", got, want)
	}

	if got, want := diagnostics.OmittedCaches, 1; got != want {
		t.Fatalf("omitted caches = %d, want %d", got, want)
	}

	encoded, err := json.Marshal(diagnostics)
	if err != nil {
		t.Fatalf("marshal diagnostics: %v", err)
	}

	for _, forbidden := range []string{"magnet:", "info_hash", "torrent_hash", "client_ip", "peer_address"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("diagnostics expose forbidden field %q: %s", forbidden, encoded)
		}
	}
}

func newRequestStrategyDiagnosticCache(debug bool) *Cache {
	cache := NewCache(64<<20, testCacheHost{sets: &settings.BTSets{EnableDebug: debug}})
	cache.pieceLength = 2 << 20
	cache.pieceCount = 100

	return cache
}

func onlyRequestStrategyCapacityItem(
	t *testing.T,
	diagnostics RequestStrategyCapacityDiagnostics,
) RequestStrategyCapacityItem {
	t.Helper()

	if got := len(diagnostics.Caches); got != 1 {
		t.Fatalf("diagnostic items = %d, want 1: %+v", got, diagnostics)
	}

	return diagnostics.Caches[0]
}
