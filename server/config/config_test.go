package config

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"

	"server/log"
	"server/settings"
)

func TestLoadConfig(t *testing.T) {
	yamlContent := `
server:
  port: "8090"
  ssl: true
  ssl_port: "8091"
dlna:
  enabled: true
  friendly_name: "Test Server"
cache:
  size_mb: 128
  preload_percent: 75
`

	tmpFile, err := os.CreateTemp("", "config-*.yml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.WriteString(yamlContent); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	_ = tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Port != "8090" {
		t.Errorf("Server.Port = %q, want %q", cfg.Server.Port, "8090")
	}

	if !cfg.Server.SSL {
		t.Error("Server.SSL = false, want true")
	}

	if !cfg.DLNA.Enabled {
		t.Error("DLNA.Enabled = false, want true")
	}

	if cfg.DLNA.FriendlyName != "Test Server" {
		t.Errorf("DLNA.FriendlyName = %q, want %q", cfg.DLNA.FriendlyName, "Test Server")
	}

	if cfg.Cache.SizeMB != 128 {
		t.Errorf("Cache.SizeMB = %d, want %d", cfg.Cache.SizeMB, 128)
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	if cfg.Server.Port != "8090" {
		t.Errorf("Server.Port = %q, want %q", cfg.Server.Port, "8090")
	}

	if cfg.Cache.SizeMB != 64 {
		t.Errorf("Cache.SizeMB = %d, want %d", cfg.Cache.SizeMB, 64)
	}

	if cfg.Cache.PreloadPercent != 50 {
		t.Errorf("Cache.PreloadPercent = %d, want %d", cfg.Cache.PreloadPercent, 50)
	}

	if cfg.Torrent.ConnectionsLimit != 25 {
		t.Errorf("Torrent.ConnectionsLimit = %d, want %d", cfg.Torrent.ConnectionsLimit, 25)
	}

	if cfg.Stream.CoreProfile != "custom" {
		t.Errorf("Stream.CoreProfile = %q, want %q", cfg.Stream.CoreProfile, "custom")
	}

	if cfg.Stream.StartupPreloadPolicy != settings.StartupPreloadPolicySkipActive {
		t.Errorf(
			"Stream.StartupPreloadPolicy = %q, want %q",
			cfg.Stream.StartupPreloadPolicy,
			settings.StartupPreloadPolicySkipActive,
		)
	}

	if cfg.Network.DisableUTP {
		t.Error("Network.DisableUTP = true, want false for compatibility defaults")
	}

	if cfg.Network.EnableLPD {
		t.Error("Network.EnableLPD = true, want false until LPD is validated by runtime A/B profiles")
	}

	if cfg.Network.LPDIPv6 {
		t.Error("Network.LPDIPv6 = true, want false by default")
	}

	if cfg.DiskCache.SyncPolicy != "periodic" {
		t.Errorf("DiskCache.SyncPolicy = %q, want %q", cfg.DiskCache.SyncPolicy, "periodic")
	}

	if cfg.Debug.Enabled {
		t.Error("Debug.Enabled = true, want false by default")
	}
}

func TestLoadConfigPreservesExplicitLPDEnable(t *testing.T) {
	yamlContent := `
network:
  enable_lpd: true
  lpd_ipv6: true
`

	tmpFile, err := os.CreateTemp("", "config-lpd-*.yml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.WriteString(yamlContent); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	_ = tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.Network.EnableLPD {
		t.Fatal("network.enable_lpd=true must enable LPD")
	}

	if !cfg.Network.LPDIPv6 {
		t.Fatal("network.lpd_ipv6=true must be preserved")
	}
}

func TestApplyNetworkSettingsMapsLPD(t *testing.T) {
	cfg := &Config{
		Network: NetworkConfig{
			EnableIPv6: true,
			EnableLPD:  true,
			LPDIPv6:    true,
		},
	}
	sets := &settings.BTSets{}

	cfg.ApplyToBTSets(sets)

	if !sets.EnableLPD {
		t.Fatal("network.enable_lpd=true must set BTSets.EnableLPD")
	}

	if !sets.LPDIPv6 {
		t.Fatal("network.lpd_ipv6=true must set BTSets.LPDIPv6")
	}
}

func TestApplyStreamSettingsMapsStartupPreloadPolicy(t *testing.T) {
	cfg := &Config{
		Stream: StreamConfig{
			StartupPreloadPolicy: settings.StartupPreloadPolicyLegacy,
		},
	}
	sets := &settings.BTSets{}

	cfg.ApplyToBTSets(sets)

	if sets.StartupPreloadPolicy != settings.StartupPreloadPolicyLegacy {
		t.Fatalf(
			"StartupPreloadPolicy = %q, want %q",
			sets.StartupPreloadPolicy,
			settings.StartupPreloadPolicyLegacy,
		)
	}
}

func TestApplyToBTSetsMaterializesTCPOnlyBalancedProfile(t *testing.T) {
	cfg := &Config{
		Stream: StreamConfig{
			CoreProfile: "tcp-only-balanced",
		},
	}
	sets := &settings.BTSets{}

	cfg.ApplyToBTSets(sets)

	if sets.CoreProfile != "tcp-only-balanced" {
		t.Fatalf("CoreProfile = %q, want tcp-only-balanced", sets.CoreProfile)
	}

	if !sets.DisableUTP {
		t.Fatal("tcp-only-balanced profile must disable uTP when applied from config")
	}

	if sets.DisableTCP {
		t.Fatal("tcp-only-balanced profile must keep TCP enabled when applied from config")
	}

	if sets.DisableDHT {
		t.Fatal("tcp-only-balanced profile must keep DHT enabled when applied from config")
	}

	if sets.DisablePEX {
		t.Fatal("tcp-only-balanced profile must keep PEX enabled when applied from config")
	}

	if sets.ConnectionsLimit != 25 {
		t.Fatalf("ConnectionsLimit = %d, want 25", sets.ConnectionsLimit)
	}
}

func TestShippedConfigDisablesFullDebugByDefault(t *testing.T) {
	data, err := os.ReadFile("config.yml")
	if err != nil {
		t.Fatalf("read shipped config.yml: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse shipped config.yml: %v", err)
	}

	if cfg.Debug.Enabled {
		t.Fatal("release config template must keep debug.enabled=false")
	}

	if cfg.Debug.ServiceOnly {
		t.Fatal("release config template must keep debug.service_only=false")
	}

	if cfg.Debug.EstablishedConnsPerTorrent != 0 {
		t.Fatal("release config template must keep debug established connections override disabled")
	}

	if cfg.Debug.TotalHalfOpenConnsOverride != 0 {
		t.Fatal("release config template must keep debug total half-open override disabled")
	}

	if cfg.Debug.TrackerBudgetOverride != 0 {
		t.Fatal("release config template must keep debug tracker budget override disabled")
	}

	if cfg.Debug.StablePeerCap != 0 {
		t.Fatal("release config template must keep debug stable peer cap disabled")
	}

	if cfg.Debug.MaxUnverifiedBytesMB != 0 {
		t.Fatal("release config template must keep debug max unverified bytes disabled")
	}

	if cfg.Stream.CoreProfile != "custom" {
		t.Fatalf("release config template must keep core_profile=custom, got %q", cfg.Stream.CoreProfile)
	}

	if cfg.Stream.StartupPreloadPolicy != settings.StartupPreloadPolicySkipActive {
		t.Fatalf(
			"release config template must keep startup_preload_policy=%q, got %q",
			settings.StartupPreloadPolicySkipActive,
			cfg.Stream.StartupPreloadPolicy,
		)
	}

	if cfg.Network.DisableUTP {
		t.Fatal("release config template must keep uTP enabled unless low-cpu is explicitly selected")
	}

	if cfg.Network.EnableLPD {
		t.Fatal("release config template must keep network.enable_lpd=false until A/B validation")
	}

	if cfg.Network.LPDIPv6 {
		t.Fatal("release config template must keep network.lpd_ipv6=false by default")
	}
}

func TestApplyDebugSettingsMapsFullDebugMode(t *testing.T) {
	t.Cleanup(func() { _ = log.SetLevel("info") })

	cfg := &Config{
		Debug: DebugConfig{
			Enabled:                    true,
			ServiceOnly:                false,
			ShowFSActiveTorr:           true,
			EstablishedConnsPerTorrent: 36,
			TotalHalfOpenConnsOverride: 500,
			TrackerBudgetOverride:      64,
			StablePeerCap:              22,
			MaxUnverifiedBytesMB:       32,
		},
	}
	sets := &settings.BTSets{}

	cfg.ApplyToBTSets(sets)

	if !sets.EnableDebug {
		t.Fatal("debug.enabled=true must set BTSets.EnableDebug")
	}

	if sets.ServiceOnlyDebug {
		t.Fatal("debug.service_only=false must keep BTSets.ServiceOnlyDebug=false")
	}

	if !sets.ShowFSActiveTorr {
		t.Fatal("debug.show_fs_active_torr=true must set BTSets.ShowFSActiveTorr")
	}

	if sets.DebugEstablishedConnsOverride != 36 {
		t.Fatalf("debug established connections override = %d, want 36", sets.DebugEstablishedConnsOverride)
	}

	if sets.DebugTotalHalfOpenConnsOverride != 500 {
		t.Fatalf("debug total half-open override = %d, want 500", sets.DebugTotalHalfOpenConnsOverride)
	}

	if sets.DebugTrackerBudgetOverride != 64 {
		t.Fatalf("debug tracker budget override = %d, want 64", sets.DebugTrackerBudgetOverride)
	}

	if sets.DebugStablePeerCap != 22 {
		t.Fatalf("debug stable peer cap = %d, want 22", sets.DebugStablePeerCap)
	}

	if sets.DebugMaxUnverifiedBytesMB != 32 {
		t.Fatalf("debug max unverified bytes MB = %d, want 32", sets.DebugMaxUnverifiedBytesMB)
	}
}

func TestApplyDebugSettingsMapsServiceOnlyMode(t *testing.T) {
	t.Cleanup(func() { _ = log.SetLevel("info") })

	cfg := &Config{
		Debug: DebugConfig{
			Enabled:     false,
			ServiceOnly: true,
		},
	}
	sets := &settings.BTSets{}

	cfg.ApplyToBTSets(sets)

	if sets.EnableDebug {
		t.Fatal("debug.enabled=false must keep BTSets.EnableDebug=false")
	}

	if !sets.ServiceOnlyDebug {
		t.Fatal("debug.service_only=true must set BTSets.ServiceOnlyDebug")
	}
}

func TestToStaticConfigPreservesDebugServiceOnly(t *testing.T) {
	cfg := &Config{
		Debug: DebugConfig{
			Enabled:                    true,
			ServiceOnly:                true,
			EstablishedConnsPerTorrent: 36,
			TotalHalfOpenConnsOverride: 500,
			TrackerBudgetOverride:      64,
			StablePeerCap:              22,
			MaxUnverifiedBytesMB:       32,
		},
	}

	staticCfg := cfg.ToStaticConfig()
	if !staticCfg.EnableDebug {
		t.Fatal("ToStaticConfig must preserve debug.enabled")
	}

	if !staticCfg.ServiceOnlyDebug {
		t.Fatal("ToStaticConfig must preserve debug.service_only")
	}

	if staticCfg.DebugEstablishedConnsOverride != 36 {
		t.Fatalf("debug established connections override = %d, want 36", staticCfg.DebugEstablishedConnsOverride)
	}

	if staticCfg.DebugTotalHalfOpenConnsOverride != 500 {
		t.Fatalf("debug total half-open override = %d, want 500", staticCfg.DebugTotalHalfOpenConnsOverride)
	}

	if staticCfg.DebugTrackerBudgetOverride != 64 {
		t.Fatalf("debug tracker budget override = %d, want 64", staticCfg.DebugTrackerBudgetOverride)
	}

	if staticCfg.DebugStablePeerCap != 22 {
		t.Fatalf("debug stable peer cap = %d, want 22", staticCfg.DebugStablePeerCap)
	}

	if staticCfg.DebugMaxUnverifiedBytesMB != 32 {
		t.Fatalf("debug max unverified bytes MB = %d, want 32", staticCfg.DebugMaxUnverifiedBytesMB)
	}
}

func TestToStaticConfigPreservesStartupPreloadPolicy(t *testing.T) {
	cfg := &Config{
		Stream: StreamConfig{
			StartupPreloadPolicy: settings.StartupPreloadPolicyLegacy,
		},
	}

	staticCfg := cfg.ToStaticConfig()
	if staticCfg.StartupPreloadPolicy != settings.StartupPreloadPolicyLegacy {
		t.Fatalf(
			"StartupPreloadPolicy = %q, want %q",
			staticCfg.StartupPreloadPolicy,
			settings.StartupPreloadPolicyLegacy,
		)
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	ResetForTest()

	_, err := Load("/nonexistent/config.yml")
	if err != nil {
		t.Fatalf("Load() unexpected error for nonexistent file: %v", err)
	}

	cfg := Get()
	if cfg == nil {
		t.Error("Get() returned nil after loading defaults")
	}
}

func TestGetConfig(t *testing.T) {
	ResetForTest()

	cfg := Get()
	if cfg == nil {
		t.Fatal("Get() returned nil")
	}

	cfg2 := Get()
	if cfg != cfg2 {
		t.Error("Get() returned different instances")
	}
}

func TestTorznabConfig(t *testing.T) {
	yamlContent := `
search:
  enable_torznab: true
  torznab_urls:
    - host: "https://api.example.com"
      key: "test-key"
      name: "Example"
`

	tmpFile, err := os.CreateTemp("", "config-*.yml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.WriteString(yamlContent); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	_ = tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.Search.EnableTorznab {
		t.Error("Search.EnableTorznab = false, want true")
	}

	if len(cfg.Search.TorznabURLs) != 1 {
		t.Fatalf("len(Search.TorznabURLs) = %d, want 1", len(cfg.Search.TorznabURLs))
	}

	if cfg.Search.TorznabURLs[0].Host != "https://api.example.com" {
		t.Errorf("TorznabURLs[0].Host = %q, want %q", cfg.Search.TorznabURLs[0].Host, "https://api.example.com")
	}
}

func ResetForTest() {
	loadedConfig = nil
}
