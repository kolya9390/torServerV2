package settings

import "testing"

func TestDBReadCacheReturnsDefensiveCopies(t *testing.T) {
	backend := newTestDB()
	backend.Set("Settings", "BitTorr", []byte(`{"a":1}`))

	cache := NewDBReadCache(backend)

	first := cache.Get("Settings", "BitTorr")
	if first == nil {
		t.Fatalf("expected data from cache")
	}
	first[0] = '['

	second := cache.Get("Settings", "BitTorr")
	if string(second) != `{"a":1}` {
		t.Fatalf("cache must return copy, got %q", string(second))
	}
}

func TestDBReadCacheListReturnsDefensiveCopies(t *testing.T) {
	backend := newTestDB()
	backend.Set("Viewed", "one", []byte(`{"i":1}`))

	cache := NewDBReadCache(backend)
	names := cache.List("Viewed")
	if len(names) != 1 {
		t.Fatalf("unexpected list length: %d", len(names))
	}
	names[0] = "mutated"

	names2 := cache.List("Viewed")
	if len(names2) != 1 || names2[0] != "one" {
		t.Fatalf("cache list must return copy, got %v", names2)
	}
}
