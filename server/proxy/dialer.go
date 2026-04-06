package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"golang.org/x/net/proxy"
)

// Dialer wraps a proxy dialer and provides methods compatible with
// anacrolix/torrent ClientConfig fields.
type Dialer struct {
	cfg *Config

	// direct is the fallback dialer for non-proxied connections.
	direct net.Dialer

	// proxyDialer is created once on first use for SOCKS/SOCKS4 proxies.
	proxyDialer proxy.Dialer
	initErr     error
}

// NewDialer creates a new proxy Dialer from configuration.
// Returns nil if cfg is nil (no proxy configured).
func NewDialer(cfg *Config) *Dialer {
	if cfg == nil {
		return nil
	}

	return &Dialer{cfg: cfg}
}

// DialContext implements a proxy-aware dial for peer and DHT connections.
// It satisfies the signature expected by torrent.ClientConfig.DialContext.
func (d *Dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if d == nil || d.cfg == nil {
		return d.direct.DialContext(ctx, network, addr)
	}

	dialer, err := d.getProxyDialer()
	if err != nil {
		return nil, fmt.Errorf("proxy dialer: %w", err)
	}

	return dialer.Dial(network, addr)
}

// HTTPProxy returns a function compatible with torrent.ClientConfig.HTTPProxy.
// It routes HTTP tracker requests through the configured proxy.
// Returns nil if no proxy is configured.
func (d *Dialer) HTTPProxy() func(*http.Request) (*url.URL, error) {
	if d == nil || d.cfg == nil {
		return nil
	}

	return func(_ *http.Request) (*url.URL, error) {
		return d.cfg.URL, nil
	}
}

// IsConfigured returns true when a proxy URL is set.
func (d *Dialer) IsConfigured() bool {
	return d != nil && d.cfg != nil && d.cfg.URL != nil
}

// getProxyDialer lazily initializes the proxy dialer on first call.
func (d *Dialer) getProxyDialer() (proxy.Dialer, error) {
	if d.proxyDialer != nil {
		return d.proxyDialer, nil
	}

	if d.initErr != nil {
		return nil, d.initErr
	}

	dialer, err := d.createProxyDialer()
	if err != nil {
		d.initErr = err
		return nil, err
	}

	d.proxyDialer = dialer

	return dialer, nil
}

// createProxyDialer builds the appropriate proxy dialer based on URL scheme.
func (d *Dialer) createProxyDialer() (proxy.Dialer, error) {
	scheme := d.cfg.URL.Scheme

	switch scheme {
	case "socks5", "socks5h":
		return proxy.SOCKS5("tcp", d.cfg.URL.Host, nil, &d.direct)
	case "socks4", "socks4a", "http", "https":
		return proxy.FromURL(d.cfg.URL, &d.direct)
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %q", scheme)
	}
}
