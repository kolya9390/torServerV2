package torr

import (
	"context"
	"log/slog"
	"net"
	"strings"
	"testing"

	"github.com/anacrolix/torrent"

	"server/settings"
	"server/torr/state"
)

type btTestSettingsProvider struct {
	sets *settings.BTSets
}

func (p btTestSettingsProvider) Get() *settings.BTSets {
	if p.sets == nil {
		return &settings.BTSets{}
	}

	cp := *p.sets

	return &cp
}

func (p btTestSettingsProvider) Set(*settings.BTSets) {}

func (p btTestSettingsProvider) ReadOnly() bool {
	return false
}

func (p btTestSettingsProvider) GetStaticConfig() settings.StaticConfig {
	return settings.StaticConfig{}
}

func (p btTestSettingsProvider) GetStoragePreferences() map[string]any {
	return map[string]any{}
}

func (p btTestSettingsProvider) SetStoragePreferences(map[string]any) error {
	return nil
}

func setupTestSettings() {
	sets := &settings.BTSets{
		CacheSize:                64 * 1024 * 1024,
		PreloadCache:             50,
		ConnectionsLimit:         10,
		TorrentDisconnectTimeout: 10,
		ReaderReadAHead:          50,
		ResponsiveMode:           true,
		RetrackersMode:           1,
	}
	settings.DefaultSettingsProvider.Set(sets)
	// Initialize Args to avoid nil pointer in configureProxy
	settings.SetArgs(&settings.ExecArgs{
		ProxyURL:  "",
		ProxyMode: "",
	})
}

func TestNewBTS(t *testing.T) {
	bts := NewBTS()
	if bts == nil {
		t.Fatal("NewBTS() returned nil")
	}

	if bts.registry == nil {
		t.Fatal("NewBTS() registry not initialized")
	}
}

func TestBTServerConnectDisconnect(t *testing.T) {
	setupTestSettings()

	bts := NewBTS()
	if err := bts.Connect(); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}

	if bts.client == nil {
		t.Fatal("client not initialized after Connect")
	}

	bts.Disconnect()

	if bts.client != nil {
		t.Fatal("client not nil after Disconnect")
	}
}

func TestBuildClientConfigDebugModes(t *testing.T) {
	tests := []struct {
		name      string
		sets      *settings.BTSets
		wantDebug bool
	}{
		{
			name: "full debug enables torrent library debug",
			sets: &settings.BTSets{
				EnableDebug:      true,
				ServiceOnlyDebug: false,
			},
			wantDebug: true,
		},
		{
			name: "service-only debug keeps torrent library debug disabled",
			sets: &settings.BTSets{
				EnableDebug:      false,
				ServiceOnlyDebug: true,
			},
			wantDebug: false,
		},
		{
			name: "both debug flags keeps torrent library debug disabled for service-only",
			sets: &settings.BTSets{
				EnableDebug:      true,
				ServiceOnlyDebug: true,
			},
			wantDebug: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bt := NewBTSWithProvidersRuntimeAndDB(
				btTestSettingsProvider{sets: tt.sets},
				settings.NewNoopArgsProvider(),
				func() settings.RuntimeState { return settings.RuntimeState{} },
				NewNoopTorrentDBStore(),
			)

			cfg := bt.buildClientConfig()
			if cfg.Debug != tt.wantDebug {
				t.Fatalf("ClientConfig.Debug = %v, want %v", cfg.Debug, tt.wantDebug)
			}

			if !cfg.DisableWebtorrent {
				t.Fatal("ClientConfig.DisableWebtorrent must stay enabled for native server streaming")
			}

			if tt.sets.ServiceOnlyDebug {
				if cfg.Logger.IsZero() {
					t.Fatal("service-only debug must install a discard torrent logger")
				}

				if cfg.Slogger == nil {
					t.Fatal("service-only debug must install a disabled slog logger")
				}

				if cfg.Slogger.Handler().Enabled(context.Background(), slog.LevelError) {
					t.Fatal("service-only debug must disable torrent slog output")
				}
			}
		})
	}
}

func TestBuildClientConfigLowCPUProfile(t *testing.T) {
	bt := NewBTSWithProvidersRuntimeAndDB(
		btTestSettingsProvider{sets: &settings.BTSets{
			CoreProfile:      "low-cpu",
			DisableUTP:       true,
			ConnectionsLimit: 12,
		}},
		settings.NewNoopArgsProvider(),
		func() settings.RuntimeState { return settings.RuntimeState{} },
		NewNoopTorrentDBStore(),
	)

	cfg := bt.buildClientConfig()
	if !cfg.DisableUTP {
		t.Fatal("low-cpu profile must disable uTP in torrent client config")
	}

	if cfg.EstablishedConnsPerTorrent != 12 {
		t.Fatalf("EstablishedConnsPerTorrent = %d, want 12", cfg.EstablishedConnsPerTorrent)
	}

	if cfg.TorrentPeersLowWater != 24 || cfg.TorrentPeersHighWater != 72 {
		t.Fatalf("peer watermarks = (%d, %d), want (24, 72)",
			cfg.TorrentPeersLowWater, cfg.TorrentPeersHighWater)
	}
}

func TestBuildClientConfigTCPOnlyBalancedProfile(t *testing.T) {
	bt := NewBTSWithProvidersRuntimeAndDB(
		btTestSettingsProvider{sets: &settings.BTSets{
			CoreProfile:      "tcp-only-balanced",
			DisableUTP:       true,
			ConnectionsLimit: 25,
		}},
		settings.NewNoopArgsProvider(),
		func() settings.RuntimeState { return settings.RuntimeState{} },
		NewNoopTorrentDBStore(),
	)

	cfg := bt.buildClientConfig()
	if !cfg.DisableUTP {
		t.Fatal("tcp-only-balanced profile must disable uTP in torrent client config")
	}

	if cfg.DisableTCP {
		t.Fatal("tcp-only-balanced profile must keep TCP enabled")
	}

	if cfg.NoDHT {
		t.Fatal("tcp-only-balanced profile must keep DHT enabled")
	}

	if cfg.DisablePEX {
		t.Fatal("tcp-only-balanced profile must keep PEX enabled")
	}

	if cfg.EstablishedConnsPerTorrent != 25 {
		t.Fatalf("EstablishedConnsPerTorrent = %d, want 25", cfg.EstablishedConnsPerTorrent)
	}

	if cfg.TorrentPeersLowWater != 50 || cfg.TorrentPeersHighWater != 500 {
		t.Fatalf("peer watermarks = (%d, %d), want balanced watermarks (50, 500)",
			cfg.TorrentPeersLowWater, cfg.TorrentPeersHighWater)
	}
}

func TestBuildClientConfigDebugEstablishedConnsOverride(t *testing.T) {
	bt := NewBTSWithProvidersRuntimeAndDB(
		btTestSettingsProvider{sets: &settings.BTSets{
			EnableDebug:                   true,
			ConnectionsLimit:              25,
			DebugEstablishedConnsOverride: 36,
		}},
		settings.NewNoopArgsProvider(),
		func() settings.RuntimeState { return settings.RuntimeState{} },
		NewNoopTorrentDBStore(),
	)

	cfg := bt.buildClientConfig()
	if cfg.EstablishedConnsPerTorrent != 36 {
		t.Fatalf("EstablishedConnsPerTorrent = %d, want 36", cfg.EstablishedConnsPerTorrent)
	}

	if cfg.TorrentPeersLowWater != 72 || cfg.TorrentPeersHighWater != 500 {
		t.Fatalf("peer watermarks = (%d, %d), want debug override watermarks (72, 500)",
			cfg.TorrentPeersLowWater, cfg.TorrentPeersHighWater)
	}
}

func TestBuildClientConfigDebugMaxUnverifiedBytes(t *testing.T) {
	tests := []struct {
		name string
		sets *settings.BTSets
		want int64
	}{
		{
			name: "disabled outside debug",
			sets: &settings.BTSets{
				DebugMaxUnverifiedBytesMB: 32,
			},
			want: torrent.NewDefaultClientConfig().MaxUnverifiedBytes,
		},
		{
			name: "zero preserves library default",
			sets: &settings.BTSets{
				EnableDebug: true,
			},
			want: torrent.NewDefaultClientConfig().MaxUnverifiedBytes,
		},
		{
			name: "debug enables measured override",
			sets: &settings.BTSets{
				EnableDebug:               true,
				DebugMaxUnverifiedBytesMB: 32,
			},
			want: 32 << 20,
		},
		{
			name: "service-only without debug does not alter request strategy",
			sets: &settings.BTSets{
				ServiceOnlyDebug:          true,
				DebugMaxUnverifiedBytesMB: 32,
			},
			want: torrent.NewDefaultClientConfig().MaxUnverifiedBytes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bt := NewBTSWithProvidersRuntimeAndDB(
				btTestSettingsProvider{sets: tt.sets},
				settings.NewNoopArgsProvider(),
				func() settings.RuntimeState { return settings.RuntimeState{} },
				NewNoopTorrentDBStore(),
			)

			cfg := bt.buildClientConfig()
			if cfg.MaxUnverifiedBytes != tt.want {
				t.Fatalf("MaxUnverifiedBytes = %d, want %d", cfg.MaxUnverifiedBytes, tt.want)
			}
		})
	}
}

func TestBTServerGetTorrent(t *testing.T) {
	setupTestSettings()

	bts := NewBTS()
	if err := bts.Connect(); err != nil {
		t.Skipf("Connect() error: %v", err)
	}

	defer bts.Disconnect()

	hash := bts.GetTorrent([20]byte{1, 2, 3})
	if hash != nil {
		t.Fatal("GetTorrent() should return nil for non-existent hash")
	}
}

func TestBTServerListTorrents(t *testing.T) {
	setupTestSettings()

	bts := NewBTS()
	if err := bts.Connect(); err != nil {
		t.Skipf("Connect() error: %v", err)
	}

	defer bts.Disconnect()

	list := bts.ListTorrents()
	if list == nil {
		t.Fatal("ListTorrents() returned nil")
	}

	if len(list) != 0 {
		t.Fatalf("ListTorrents() expected 0, got %d", len(list))
	}
}

func TestBTServerRemoveTorrent(t *testing.T) {
	setupTestSettings()

	bts := NewBTS()
	if err := bts.Connect(); err != nil {
		t.Skipf("Connect() error: %v", err)
	}

	defer bts.Disconnect()

	// Remove non-existent torrent should return false
	removed := bts.RemoveTorrent([20]byte{1, 2, 3})
	if removed {
		t.Fatal("RemoveTorrent() should return false for non-existent hash")
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
	}
	for _, tt := range tests {
		parsed := net.ParseIP(tt.ip)
		if parsed == nil {
			t.Fatalf("failed to parse IP %q", tt.ip)
		}

		got := isPrivateIP(parsed)
		if got != tt.want {
			t.Errorf("isPrivateIP(%q) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestTorrentStateTransitions(t *testing.T) {
	setupTestSettings()

	bts := NewBTS()
	if err := bts.Connect(); err != nil {
		t.Skipf("Connect() error: %v", err)
	}

	defer bts.Disconnect()

	spec := &torrent.TorrentSpec{
		AddTorrentOpts: torrent.AddTorrentOpts{
			InfoHash: [20]byte{0xAA, 0xBB, 0xCC},
		},
		DisplayName: "Test Torrent",
		Trackers:    [][]string{{"udp://tracker.example.com:1337"}},
	}

	torr, err := NewTorrent(spec, bts)
	if err != nil {
		t.Fatalf("NewTorrent() error: %v", err)
	}

	if torr == nil {
		t.Fatal("NewTorrent() returned nil")
	}

	if torr.Stat != state.TorrentAdded {
		t.Errorf("Torrent stat = %v, want %v", torr.Stat, state.TorrentAdded)
	}
}

func TestPeerWatermarks(t *testing.T) {
	tests := []struct {
		name      string
		effective int
		wantLow   int
		wantHigh  int
	}{
		{name: "defaults", effective: 0, wantLow: 50, wantHigh: 500},
		{name: "low connections floor", effective: 8, wantLow: 50, wantHigh: 500},
		{name: "medium", effective: 25, wantLow: 50, wantHigh: 500},
		{name: "high", effective: 80, wantLow: 160, wantHigh: 800},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			low, high := peerWatermarks(tt.effective)
			if low != tt.wantLow || high != tt.wantHigh {
				t.Fatalf("peerWatermarks(%d) = (%d, %d), want (%d, %d)", tt.effective, low, high, tt.wantLow, tt.wantHigh)
			}

			if high < low+50 {
				t.Fatalf("high watermark must provide headroom: low=%d high=%d effective=%d", low, high, tt.effective)
			}
		})
	}
}

func TestEffectiveEstablishedConns(t *testing.T) {
	tests := []struct {
		name         string
		userLimit    int
		defaultConns int
		want         int
	}{
		{name: "uses library default when unset", userLimit: 0, defaultConns: 50, want: 50},
		{name: "honors explicit user limit below default", userLimit: 25, defaultConns: 50, want: 25},
		{name: "keeps higher user limit", userLimit: 80, defaultConns: 50, want: 80},
		{name: "fallback default when invalid", userLimit: 0, defaultConns: 0, want: 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveEstablishedConns(tt.userLimit, tt.defaultConns); got != tt.want {
				t.Fatalf("effectiveEstablishedConns(%d, %d) = %d, want %d", tt.userLimit, tt.defaultConns, got, tt.want)
			}
		})
	}
}

func TestConnectionPolicyForSettings(t *testing.T) {
	tests := []struct {
		name               string
		sets               *settings.BTSets
		wantEffectiveConns int
		wantLowWater       int
		wantHighWater      int
		wantTrackers       int
		wantLowCPU         bool
		wantDebugOverride  int
	}{
		{
			name:               "custom profile honors configured low limit",
			sets:               &settings.BTSets{ConnectionsLimit: 12},
			wantEffectiveConns: 12,
			wantLowWater:       50,
			wantHighWater:      500,
			wantTrackers:       8,
		},
		{
			name:               "compatibility high limit preserved",
			sets:               &settings.BTSets{ConnectionsLimit: 96},
			wantEffectiveConns: 96,
			wantLowWater:       192,
			wantHighWater:      960,
			wantTrackers:       24,
		},
		{
			name:               "low cpu profile honors measured low budget",
			sets:               &settings.BTSets{CoreProfile: "low-cpu", DisableUTP: true, ConnectionsLimit: 12},
			wantEffectiveConns: 12,
			wantLowWater:       24,
			wantHighWater:      72,
			wantTrackers:       8,
			wantLowCPU:         true,
		},
		{
			name:               "low cpu profile unset limit uses conservative budget",
			sets:               &settings.BTSets{CoreProfile: "low-cpu", DisableUTP: true},
			wantEffectiveConns: 24,
			wantLowWater:       48,
			wantHighWater:      144,
			wantTrackers:       8,
			wantLowCPU:         true,
		},
		{
			name: "debug override isolates established connections",
			sets: &settings.BTSets{
				EnableDebug:                   true,
				DebugEstablishedConnsOverride: 36,
				ConnectionsLimit:              25,
			},
			wantEffectiveConns: 36,
			wantLowWater:       72,
			wantHighWater:      500,
			wantTrackers:       16,
			wantDebugOverride:  36,
		},
		{
			name: "debug override ignored without debug mode",
			sets: &settings.BTSets{
				DebugEstablishedConnsOverride: 24,
				ConnectionsLimit:              25,
			},
			wantEffectiveConns: 25,
			wantLowWater:       50,
			wantHighWater:      500,
			wantTrackers:       16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := connectionPolicyForSettings(tt.sets, 50)
			if got.effectiveConns != tt.wantEffectiveConns {
				t.Fatalf("effectiveConns = %d, want %d", got.effectiveConns, tt.wantEffectiveConns)
			}

			if got.peerLowWater != tt.wantLowWater || got.peerHighWater != tt.wantHighWater {
				t.Fatalf("watermarks = (%d, %d), want (%d, %d)",
					got.peerLowWater, got.peerHighWater, tt.wantLowWater, tt.wantHighWater)
			}

			if got.trackerBudget != tt.wantTrackers {
				t.Fatalf("trackerBudget = %d, want %d", got.trackerBudget, tt.wantTrackers)
			}

			if got.lowCPU != tt.wantLowCPU {
				t.Fatalf("lowCPU = %v, want %v", got.lowCPU, tt.wantLowCPU)
			}

			if got.debugOverride != tt.wantDebugOverride {
				t.Fatalf("debugOverride = %d, want %d", got.debugOverride, tt.wantDebugOverride)
			}
		})
	}
}

func TestApplyTrackerPolicyPreservesMagnetTrackersBeforeDefaults(t *testing.T) {
	spec := &torrent.TorrentSpec{
		Trackers: [][]string{{
			"udp://magnet-a.example.com:6969/announce",
			"UDP://MAGNET-A.EXAMPLE.COM:6969/announce",
			"wss://webtorrent.example.com",
		}},
	}
	sets := &settings.BTSets{
		RetrackersMode:   1,
		ConnectionsLimit: 12,
		CoreProfile:      "low-cpu",
	}

	applyTrackerPolicy(spec, sets, true, []string{"https://file-tracker.example.com/announce"})

	if len(spec.Trackers) == 0 || len(spec.Trackers[0]) == 0 {
		t.Fatal("expected tracker tiers")
	}

	if got := spec.Trackers[0][0]; got != "udp://magnet-a.example.com:6969/announce" {
		t.Fatalf("first tracker = %q, want magnet tracker first", got)
	}

	total := countSpecTrackers(spec)
	if total != 8 {
		t.Fatalf("tracker count = %d, want low-cpu budget cap 8", total)
	}

	for _, tier := range spec.Trackers {
		for _, tracker := range tier {
			if strings.HasPrefix(strings.ToLower(tracker), "ws") {
				t.Fatalf("native tracker policy must drop WebTorrent tracker %q", tracker)
			}
		}
	}
}

func TestApplyTrackerPolicyModes(t *testing.T) {
	tests := []struct {
		name       string
		mode       int
		wantFirst  string
		wantMagnet bool
	}{
		{name: "remove retrackers keeps no trackers", mode: 2},
		{name: "replace retrackers uses defaults", mode: 3, wantFirst: "http://retracker.local/announce"},
		{name: "unset mode preserves magnet only", mode: 0, wantFirst: "udp://magnet.example.com:6969/announce", wantMagnet: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &torrent.TorrentSpec{
				Trackers: [][]string{{"udp://magnet.example.com:6969/announce"}},
			}
			sets := &settings.BTSets{RetrackersMode: tt.mode, ConnectionsLimit: 12}

			applyTrackerPolicy(spec, sets, true, nil)

			if tt.wantFirst == "" {
				if len(spec.Trackers) != 0 {
					t.Fatalf("trackers = %v, want none", spec.Trackers)
				}

				return
			}

			if len(spec.Trackers) == 0 || len(spec.Trackers[0]) == 0 {
				t.Fatalf("trackers = %v, want first %q", spec.Trackers, tt.wantFirst)
			}

			if got := spec.Trackers[0][0]; got != tt.wantFirst {
				t.Fatalf("first tracker = %q, want %q", got, tt.wantFirst)
			}

			if !tt.wantMagnet && trackerListContains(spec.Trackers, "udp://magnet.example.com:6969/announce") {
				t.Fatalf("magnet tracker should not be preserved for mode %d", tt.mode)
			}
		})
	}
}

func countSpecTrackers(spec *torrent.TorrentSpec) int {
	total := 0
	for _, tier := range spec.Trackers {
		total += len(tier)
	}

	return total
}

func trackerListContains(trackers [][]string, want string) bool {
	for _, tier := range trackers {
		for _, tracker := range tier {
			if tracker == want {
				return true
			}
		}
	}

	return false
}

func TestActivePlaybackTorrents(t *testing.T) {
	t.Parallel()

	bts := NewBTS()
	bts.registry.LoadOrStore([20]byte{1}, &Torrent{})
	bts.registry.LoadOrStore([20]byte{2}, &Torrent{})
	bts.registry.LoadOrStore([20]byte{3}, nil)

	if got, want := bts.ActivePlaybackTorrents(), 1; got != want {
		t.Fatalf("ActivePlaybackTorrents() = %d, want %d", got, want)
	}

	empty := NewBTS()
	if got, want := empty.ActivePlaybackTorrents(), 1; got != want {
		t.Fatalf("ActivePlaybackTorrents on empty server = %d, want %d", got, want)
	}
}
