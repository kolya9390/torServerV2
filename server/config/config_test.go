package config

import (
	"os"
	"testing"
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

	if cfg.DiskCache.SyncPolicy != "periodic" {
		t.Errorf("DiskCache.SyncPolicy = %q, want %q", cfg.DiskCache.SyncPolicy, "periodic")
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
