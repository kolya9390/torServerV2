package metrics

import (
	"testing"

	"server/settings"
	"server/torr/storage/torrstor"
)

type metricsTestSettingsProvider struct {
	sets *settings.BTSets
}

func (p metricsTestSettingsProvider) Get() *settings.BTSets {
	return p.sets
}

func (metricsTestSettingsProvider) Set(*settings.BTSets) {}

func (metricsTestSettingsProvider) ReadOnly() bool {
	return true
}

func (metricsTestSettingsProvider) GetStaticConfig() settings.StaticConfig {
	return settings.StaticConfig{}
}

func (metricsTestSettingsProvider) GetStoragePreferences() map[string]any {
	return map[string]any{}
}

func (metricsTestSettingsProvider) SetStoragePreferences(map[string]any) error {
	return nil
}

func TestTorrentConnectionPolicySnapshot(t *testing.T) {
	sets := &settings.BTSets{
		ConnectionsLimit:  25,
		DisableDHT:        true,
		DisablePEX:        false,
		DisableTCP:        true,
		DisableUTP:        false,
		DisableUpload:     true,
		DownloadRateLimit: 512,
		UploadRateLimit:   128,
	}

	got := torrentConnectionPolicySnapshot(sets)

	assertIntMetric(t, got, "connections_limit", 25)
	assertIntMetric(t, got, "effective_conns", 25)
	assertIntMetric(t, got, "peer_low_water", 50)
	assertIntMetric(t, got, "peer_high_water", 500)
	assertIntMetric(t, got, "tracker_budget", 16)
	assertIntMetric(t, got, "debug_established_conns_override", 0)
	assertInt64Metric(t, got, "debug_max_unverified_bytes", 0)
	assertBoolMetric(t, got, "low_cpu_profile", false)
	assertBoolMetric(t, got, "dht_enabled", false)
	assertBoolMetric(t, got, "pex_enabled", true)
	assertBoolMetric(t, got, "tcp_enabled", false)
	assertBoolMetric(t, got, "utp_enabled", true)
	assertBoolMetric(t, got, "upload_enabled", false)
	assertIntMetric(t, got, "download_rate_limit_kbs", 512)
	assertIntMetric(t, got, "upload_rate_limit_kbs", 128)
}

func TestTorrentConnectionPolicySnapshotNilSettings(t *testing.T) {
	got := torrentConnectionPolicySnapshot(nil)

	assertIntMetric(t, got, "connections_limit", 0)
	assertIntMetric(t, got, "effective_conns", 50)
	assertIntMetric(t, got, "peer_low_water", 100)
	assertIntMetric(t, got, "peer_high_water", 500)
	assertIntMetric(t, got, "tracker_budget", 16)
	assertIntMetric(t, got, "debug_established_conns_override", 0)
	assertInt64Metric(t, got, "debug_max_unverified_bytes", 0)
	assertBoolMetric(t, got, "low_cpu_profile", false)
	assertBoolMetric(t, got, "dht_enabled", true)
	assertBoolMetric(t, got, "pex_enabled", true)
	assertBoolMetric(t, got, "tcp_enabled", true)
	assertBoolMetric(t, got, "utp_enabled", true)
	assertBoolMetric(t, got, "upload_enabled", true)
}

func TestTorrentConnectionPolicySnapshotLowCPUProfile(t *testing.T) {
	sets := &settings.BTSets{
		CoreProfile:      "low-cpu",
		ConnectionsLimit: 12,
		DisableUTP:       true,
	}

	got := torrentConnectionPolicySnapshot(sets)

	assertIntMetric(t, got, "connections_limit", 12)
	assertIntMetric(t, got, "effective_conns", 12)
	assertIntMetric(t, got, "peer_low_water", 24)
	assertIntMetric(t, got, "peer_high_water", 72)
	assertIntMetric(t, got, "tracker_budget", 8)
	assertIntMetric(t, got, "debug_established_conns_override", 0)
	assertInt64Metric(t, got, "debug_max_unverified_bytes", 0)
	assertBoolMetric(t, got, "low_cpu_profile", true)
	assertBoolMetric(t, got, "utp_enabled", false)
}

func TestTorrentConnectionPolicySnapshotDebugEstablishedConnsOverride(t *testing.T) {
	sets := &settings.BTSets{
		EnableDebug:                   true,
		ConnectionsLimit:              25,
		DebugEstablishedConnsOverride: 36,
	}

	got := torrentConnectionPolicySnapshot(sets)

	assertIntMetric(t, got, "connections_limit", 25)
	assertIntMetric(t, got, "effective_conns", 36)
	assertIntMetric(t, got, "peer_low_water", 72)
	assertIntMetric(t, got, "peer_high_water", 500)
	assertIntMetric(t, got, "tracker_budget", 16)
	assertIntMetric(t, got, "debug_established_conns_override", 36)
	assertInt64Metric(t, got, "debug_max_unverified_bytes", 0)
	assertBoolMetric(t, got, "utp_enabled", true)
}

func TestTorrentConnectionPolicySnapshotDebugMaxUnverifiedBytes(t *testing.T) {
	sets := &settings.BTSets{
		EnableDebug:               true,
		DebugMaxUnverifiedBytesMB: 32,
	}

	got := torrentConnectionPolicySnapshot(sets)

	assertInt64Metric(t, got, "debug_max_unverified_bytes", 32<<20)
}

func TestTorrentConnectionPolicySnapshotIgnoresMaxUnverifiedOutsideDebug(t *testing.T) {
	sets := &settings.BTSets{
		DebugMaxUnverifiedBytesMB: 32,
	}

	got := torrentConnectionPolicySnapshot(sets)

	assertInt64Metric(t, got, "debug_max_unverified_bytes", 0)
}

func TestTorrentRuntimeSnapshotNilBackend(t *testing.T) {
	got := torrentRuntimeSnapshot(nil)

	assertIntMetric(t, got, "active_peers", 0)
	assertIntMetric(t, got, "total_peers", 0)
	assertIntMetric(t, got, "pending_peers", 0)
	assertIntMetric(t, got, "half_open_peers", 0)
	assertIntMetric(t, got, "connected_seeders", 0)
	assertIntMetric(t, got, "active_readers", 0)

	torrents, ok := got["torrents"].([]map[string]any)
	if !ok {
		t.Fatalf("torrents type = %T, want []map[string]any", got["torrents"])
	}

	if len(torrents) != 0 {
		t.Fatalf("torrents len = %d, want 0", len(torrents))
	}
}

func TestShouldStartRuntimeUpdater(t *testing.T) {
	tests := []struct {
		name     string
		provider settings.SettingsProvider
		want     bool
	}{
		{
			name:     "nil provider disables updater",
			provider: nil,
			want:     false,
		},
		{
			name:     "nil settings disables updater",
			provider: metricsTestSettingsProvider{},
			want:     false,
		},
		{
			name: "debug disabled disables updater",
			provider: metricsTestSettingsProvider{
				sets: &settings.BTSets{EnableDebug: false},
			},
			want: false,
		},
		{
			name: "debug enabled enables updater",
			provider: metricsTestSettingsProvider{
				sets: &settings.BTSets{EnableDebug: true},
			},
			want: true,
		},
		{
			name: "service-only debug still enables updater",
			provider: metricsTestSettingsProvider{
				sets: &settings.BTSets{EnableDebug: true, ServiceOnlyDebug: true},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldStartRuntimeUpdater(tt.provider); got != tt.want {
				t.Fatalf("shouldStartRuntimeUpdater() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequestStrategyPressureSnapshotLow(t *testing.T) {
	runtime := map[string]any{
		"active_peers":      9,
		"total_peers":       11,
		"connected_seeders": 9,
		"active_readers":    1,
		"torrents": []map[string]any{
			{"download_speed": 1024},
		},
	}
	cacheStats := torrstor.CacheStats{
		LogicalFilledBytes:      80 * 1024 * 1024,
		ConfiguredCapacityBytes: 64 * 1024 * 1024,
		PiecesCount:             11_464,
		InMemoryChunks:          4_876,
		Misses:                  0,
	}

	got := requestStrategyPressureSnapshot(runtime, cacheStats)

	assertStringMetric(t, got, "level", "low")
	assertInt64Metric(t, got, "score", 103_176)
	assertIntMetric(t, got, "active_peers", 9)
	assertIntMetric(t, got, "active_readers", 1)
	assertInt64Metric(t, got, "download_speed", 1024)
	assertInt64Metric(t, got, "cache_fill_percent", 125)
}

func TestRequestStrategyPressureSnapshotHigh(t *testing.T) {
	runtime := map[string]any{
		"active_peers":      90,
		"active_readers":    2,
		"connected_seeders": 80,
		"torrents": []map[string]any{
			{"download_speed": 2048},
			{"download_speed": 4096},
		},
	}
	cacheStats := torrstor.CacheStats{
		ConfiguredCapacityBytes: 128 * 1024 * 1024,
		LogicalOverheadBytes:    32 * 1024 * 1024,
		PiecesCount:             20_000,
	}

	got := requestStrategyPressureSnapshot(runtime, cacheStats)

	assertStringMetric(t, got, "level", "high")
	assertInt64Metric(t, got, "score", 3_600_000)
	assertInt64Metric(t, got, "download_speed", 6144)
	assertInt64Metric(t, got, "cache_overhead_percent", 25)
}

func assertIntMetric(t *testing.T, metrics map[string]any, key string, want int) {
	t.Helper()

	got, ok := metrics[key].(int)
	if !ok {
		t.Fatalf("%s type = %T, want int", key, metrics[key])
	}

	if got != want {
		t.Fatalf("%s = %d, want %d", key, got, want)
	}
}

func assertInt64Metric(t *testing.T, metrics map[string]any, key string, want int64) {
	t.Helper()

	got, ok := metrics[key].(int64)
	if !ok {
		t.Fatalf("%s type = %T, want int64", key, metrics[key])
	}

	if got != want {
		t.Fatalf("%s = %d, want %d", key, got, want)
	}
}

func assertBoolMetric(t *testing.T, metrics map[string]any, key string, want bool) {
	t.Helper()

	got, ok := metrics[key].(bool)
	if !ok {
		t.Fatalf("%s type = %T, want bool", key, metrics[key])
	}

	if got != want {
		t.Fatalf("%s = %v, want %v", key, got, want)
	}
}

func assertStringMetric(t *testing.T, metrics map[string]any, key string, want string) {
	t.Helper()

	got, ok := metrics[key].(string)
	if !ok {
		t.Fatalf("%s type = %T, want string", key, metrics[key])
	}

	if got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}
