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
		{name: "floors low user limit to default", userLimit: 25, defaultConns: 50, want: 50},
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
