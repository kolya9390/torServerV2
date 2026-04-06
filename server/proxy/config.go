// Package proxy provides upstream-compatible proxy configuration for BitTorrent clients.
// It supports SOCKS5, SOCKS4, HTTP, and HTTPS proxies with three routing modes:
// tracker-only, peers-only, and full traffic.
package proxy

import (
	"fmt"
	"net/url"
)

// Mode defines how proxy traffic is routed.
type Mode int

const (
	// ModeTracker proxies only HTTP tracker requests.
	// DHT and peer connections bypass the proxy.
	ModeTracker Mode = iota

	// ModePeers proxies peer connections and DHT traffic.
	// HTTP tracker requests also go through the proxy.
	ModePeers

	// ModeFull proxies all BitTorrent traffic including tracker, DHT, peers, and webtorrent.
	ModeFull
)

// ParseMode converts a string configuration value to a Mode.
// Returns ModeTracker for unknown values.
func ParseMode(s string) Mode {
	switch s {
	case "peers":
		return ModePeers
	case "full":
		return ModeFull
	default:
		return ModeTracker
	}
}

// Config holds proxy connection settings.
type Config struct {
	URL  *url.URL
	Mode Mode
}

// Validate checks that the proxy URL uses a supported scheme.
func (c *Config) Validate() error {
	if c.URL == nil {
		return nil // no proxy configured
	}

	switch c.URL.Scheme {
	case "socks5", "socks5h", "socks4", "socks4a", "http", "https":
		return nil
	default:
		return fmt.Errorf("unsupported proxy scheme: %q (supported: http, https, socks4, socks4a, socks5, socks5h)", c.URL.Scheme)
	}
}

// NewConfig parses a proxy URL string and mode into a validated Config.
func NewConfig(proxyURL, modeStr string) (*Config, error) {
	if proxyURL == "" {
		return nil, nil // no proxy
	}

	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}

	cfg := &Config{
		URL:  u,
		Mode: ParseMode(modeStr),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}
