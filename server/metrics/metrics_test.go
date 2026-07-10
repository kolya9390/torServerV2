package metrics

import (
	"encoding/json"
	"strings"
	"testing"

	"server/settings"
	"server/torr"
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
	assertIntMetric(t, got, "total_half_open_conns", 200)
	assertIntMetric(t, got, "tracker_budget", 16)
	assertIntMetric(t, got, "debug_established_conns_override", 0)
	assertIntMetric(t, got, "debug_total_half_open_conns_override", 0)
	assertIntMetric(t, got, "debug_tracker_budget_override", 0)
	assertIntMetric(t, got, "debug_stable_peer_cap", 0)
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
	assertIntMetric(t, got, "total_half_open_conns", 400)
	assertIntMetric(t, got, "tracker_budget", 16)
	assertIntMetric(t, got, "debug_established_conns_override", 0)
	assertIntMetric(t, got, "debug_total_half_open_conns_override", 0)
	assertIntMetric(t, got, "debug_tracker_budget_override", 0)
	assertIntMetric(t, got, "debug_stable_peer_cap", 0)
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
	assertIntMetric(t, got, "total_half_open_conns", 200)
	assertIntMetric(t, got, "tracker_budget", 8)
	assertIntMetric(t, got, "debug_established_conns_override", 0)
	assertIntMetric(t, got, "debug_total_half_open_conns_override", 0)
	assertIntMetric(t, got, "debug_tracker_budget_override", 0)
	assertIntMetric(t, got, "debug_stable_peer_cap", 0)
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
	assertIntMetric(t, got, "total_half_open_conns", 288)
	assertIntMetric(t, got, "tracker_budget", 16)
	assertIntMetric(t, got, "debug_established_conns_override", 36)
	assertIntMetric(t, got, "debug_total_half_open_conns_override", 0)
	assertIntMetric(t, got, "debug_tracker_budget_override", 0)
	assertIntMetric(t, got, "debug_stable_peer_cap", 0)
	assertInt64Metric(t, got, "debug_max_unverified_bytes", 0)
	assertBoolMetric(t, got, "utp_enabled", true)
}

func TestTorrentConnectionPolicySnapshotDebugPeerAcquisitionOverrides(t *testing.T) {
	sets := &settings.BTSets{
		EnableDebug:                     true,
		ConnectionsLimit:                25,
		DebugTotalHalfOpenConnsOverride: 500,
		DebugTrackerBudgetOverride:      64,
		DebugStablePeerCap:              22,
	}

	got := torrentConnectionPolicySnapshot(sets)

	assertIntMetric(t, got, "total_half_open_conns", 500)
	assertIntMetric(t, got, "tracker_budget", 64)
	assertIntMetric(t, got, "debug_total_half_open_conns_override", 500)
	assertIntMetric(t, got, "debug_tracker_budget_override", 64)
	assertIntMetric(t, got, "debug_stable_peer_cap", 22)
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

func TestTorrentRuntimeItemIncludesPeerCapDiagnostics(t *testing.T) {
	got := torrentRuntimeItem(3, torr.TorrentRuntimeMetrics{
		RuntimeID:        17,
		ActivePeers:      22,
		TotalPeers:       30,
		PendingPeers:     2,
		HalfOpenPeers:    1,
		ConnectedSeeders: 21,
		MaxEstablished:   22,
		ActiveReaders:    1,
		TotalReaders:     1,
		OldestReaderMS:   61_000,
		DownloadSpeed:    8 << 20,
	})

	assertIntMetric(t, got, "index", 3)
	assertUint64Metric(t, got, "torrent_id", 17)
	assertIntMetric(t, got, "active_peers", 22)
	assertIntMetric(t, got, "max_established_conns", 22)
	assertIntMetric(t, got, "active_readers", 1)
	assertInt64Metric(t, got, "oldest_reader_age_ms", 61_000)
	assertInt64Metric(t, got, "download_speed", 8<<20)
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
		ResidentPieces:          11_464,
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
		ResidentPieces:          20_000,
	}

	got := requestStrategyPressureSnapshot(runtime, cacheStats)

	assertStringMetric(t, got, "level", "high")
	assertInt64Metric(t, got, "score", 3_600_000)
	assertInt64Metric(t, got, "download_speed", 6144)
	assertInt64Metric(t, got, "cache_overhead_percent", 25)
}

func TestStreamSessionSnapshotClassifiesExtraCacheReaders(t *testing.T) {
	runtime := map[string]any{
		"torrents": []map[string]any{
			{
				"torrent_id":           uint64(1),
				"active_readers":       2,
				"total_readers":        2,
				"idle_readers":         0,
				"oldest_reader_age_ms": int64(12_000),
				"newest_reader_age_ms": int64(2_000),
				"max_reader_idle_ms":   int64(1_000),
				"preload_active":       false,
				"preloaded_bytes":      int64(0),
				"preload_target_bytes": int64(0),
			},
			{
				"torrent_id":           uint64(2),
				"active_readers":       1,
				"total_readers":        1,
				"idle_readers":         0,
				"oldest_reader_age_ms": int64(5_000),
				"newest_reader_age_ms": int64(5_000),
				"max_reader_idle_ms":   int64(500),
				"preload_active":       true,
				"preloaded_bytes":      int64(8 << 20),
				"preload_target_bytes": int64(32 << 20),
			},
		},
	}

	got := streamSessionSnapshotFromSources(
		runtime,
		map[uint64]int{1: 1, 2: 1},
		map[uint64]int{1: 1, 2: 1},
		2,
		2,
	)

	assertIntMetric(t, got, "active_playback_sessions", 2)
	assertIntMetric(t, got, "active_unique_playback_torrents", 2)
	assertIntMetric(t, got, "active_delivery_streams", 2)
	assertIntMetric(t, got, "active_cache_readers", 3)
	assertIntMetric(t, got, "helper_readers_estimate", 1)
	assertIntMetric(t, got, "preload_active_torrents", 1)
	assertInt64Metric(t, got, "oldest_reader_age_ms", 12_000)
	assertInt64Metric(t, got, "max_reader_idle_ms", 1_000)
	assertStringMetric(
		t,
		got,
		"interpretation",
		"cache reader pressure is higher than playback session demand; inspect range/reconnect helper readers",
	)

	first := streamSessionTorrentByID(t, got, 1)
	assertIntMetric(t, first, "playback_sessions", 1)
	assertIntMetric(t, first, "cache_readers", 2)
	assertIntMetric(t, first, "helper_readers_estimate", 1)
	assertStringMetric(t, first, "classification", "extra_cache_readers")

	second := streamSessionTorrentByID(t, got, 2)
	assertIntMetric(t, second, "playback_sessions", 1)
	assertIntMetric(t, second, "cache_readers", 1)
	assertIntMetric(t, second, "helper_readers_estimate", 0)
	assertBoolMetric(t, second, "preload_active", true)
	assertStringMetric(t, second, "classification", "playback_aligned")
}

func TestStreamSessionSnapshotPrivacySafeShape(t *testing.T) {
	runtime := map[string]any{
		"torrents": []map[string]any{
			{
				"torrent_id":     uint64(7),
				"active_readers": 1,
				"total_readers":  1,
			},
		},
	}

	got := streamSessionSnapshotFromSources(runtime, map[uint64]int{7: 1}, map[uint64]int{7: 1}, 1, 1)
	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal stream session snapshot: %v", err)
	}

	for _, forbidden := range []string{"hash", "title", "path", "ip", "query"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("stream session snapshot leaks forbidden key %q: %s", forbidden, payload)
		}
	}
}

func TestStreamSessionSnapshotEmptySources(t *testing.T) {
	got := streamSessionSnapshotFromSources(nil, nil, nil, 0, 0)

	assertIntMetric(t, got, "active_playback_sessions", 0)
	assertIntMetric(t, got, "active_unique_playback_torrents", 0)
	assertIntMetric(t, got, "active_delivery_streams", 0)
	assertIntMetric(t, got, "active_cache_readers", 0)
	assertIntMetric(t, got, "helper_readers_estimate", 0)
	assertStringMetric(
		t,
		got,
		"interpretation",
		"playback session demand and cache reader pressure are aligned",
	)

	torrents, ok := got["torrents"].([]map[string]any)
	if !ok {
		t.Fatalf("torrents type = %T, want []map[string]any", got["torrents"])
	}

	if len(torrents) != 0 {
		t.Fatalf("torrents len = %d, want 0", len(torrents))
	}
}

func streamSessionTorrentByID(t *testing.T, snapshot map[string]any, torrentID uint64) map[string]any {
	t.Helper()

	torrents, ok := snapshot["torrents"].([]map[string]any)
	if !ok {
		t.Fatalf("torrents type = %T, want []map[string]any", snapshot["torrents"])
	}

	for _, torrent := range torrents {
		if uint64Metric(torrent, "torrent_id") == torrentID {
			return torrent
		}
	}

	t.Fatalf("torrent_id %d not found in %v", torrentID, torrents)

	return nil
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

func assertUint64Metric(t *testing.T, metrics map[string]any, key string, want uint64) {
	t.Helper()

	got, ok := metrics[key].(uint64)
	if !ok {
		t.Fatalf("%s type = %T, want uint64", key, metrics[key])
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
