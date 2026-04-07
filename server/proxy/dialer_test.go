package proxy

import (
	"net/url"
	"testing"
)

func TestParseMode(t *testing.T) {
	tests := []struct {
		input    string
		expected Mode
	}{
		{"tracker", ModeTracker},
		{"", ModeTracker},
		{"peers", ModePeers},
		{"full", ModeFull},
		{"unknown", ModeTracker},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ParseMode(tt.input); got != tt.expected {
				t.Errorf("ParseMode(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		mode    string
		wantErr bool
	}{
		{"empty URL", "", "tracker", false},
		{"valid SOCKS5", "socks5://127.0.0.1:1080", "full", false},
		{"valid SOCKS5h", "socks5h://proxy.example.com:1080", "peers", false},
		{"valid SOCKS4", "socks4://127.0.0.1:1080", "tracker", false},
		{"valid SOCKS4a", "socks4a://127.0.0.1:1080", "full", false},
		{"valid HTTP", "http://127.0.0.1:3128", "tracker", false},
		{"valid HTTPS", "https://proxy.example.com:443", "peers", false},
		{"invalid URL", "://invalid", "", true},
		{"invalid scheme", "ftp://proxy.example.com", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := NewConfig(tt.url, tt.mode)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)

				return
			}

			if tt.url == "" {
				if cfg != nil {
					t.Error("expected nil config for empty URL")
				}

				return
			}

			if cfg == nil {
				t.Fatal("expected non-nil config")
			}

			if cfg.URL.String() != tt.url {
				t.Errorf("URL = %q, want %q", cfg.URL.String(), tt.url)
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		scheme  string
		wantErr bool
	}{
		{"nil URL", "", false},
		{"valid socks5", "socks5", false},
		{"valid http", "http", false},
		{"valid https", "https", false},
		{"valid socks5h", "socks5h", false},
		{"invalid ftp", "ftp", true},
		{"invalid git", "git", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			if tt.scheme != "" {
				cfg.URL = &url.URL{Scheme: tt.scheme, Host: "proxy.example.com:1080"}
			}

			err := cfg.Validate()
			if tt.wantErr && err == nil {
				t.Error("expected validation error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestDialerNil(t *testing.T) {
	var d *Dialer

	if d.IsConfigured() {
		t.Error("nil dialer should not be configured")
	}

	if d.HTTPProxy() != nil {
		t.Error("nil dialer HTTPProxy should return nil")
	}
}

func TestDialerNoProxy(t *testing.T) {
	d := NewDialer(nil)
	if d.IsConfigured() {
		t.Error("dialer with nil config should not be configured")
	}

	if d.HTTPProxy() != nil {
		t.Error("HTTPProxy should return nil for nil config")
	}
}

func TestDialerConfigured(t *testing.T) {
	cfg, err := NewConfig("socks5://127.0.0.1:1080", "full")
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	d := NewDialer(cfg)
	if !d.IsConfigured() {
		t.Error("dialer should be configured")
	}

	if d.HTTPProxy() == nil {
		t.Error("HTTPProxy should return non-nil function")
	}
}
