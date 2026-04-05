package torr

import (
	"net"
	"testing"

	"github.com/anacrolix/torrent"

	"server/settings"
	"server/torr/state"
)

func setupTestSettings() {
	sets := &settings.BTSets{
		CacheSize:               64 * 1024 * 1024,
		PreloadCache:            50,
		ConnectionsLimit:        10,
		TorrentDisconnectTimeout: 10,
		ReaderReadAHead:         50,
		ResponsiveMode:          true,
		RetrackersMode:          1,
	}
	settings.BTsets = sets
	// Initialize Args to avoid nil pointer in configureProxy
	settings.Args = &settings.ExecArgs{
		ProxyURL:  "",
		ProxyMode: "",
	}
}

func TestNewBTS(t *testing.T) {
	bts := NewBTS()
	if bts == nil {
		t.Fatal("NewBTS() returned nil")
	}
	if bts.torrents == nil {
		t.Fatal("NewBTS() torrents not initialized")
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
		InfoHash:    [20]byte{0xAA, 0xBB, 0xCC},
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
