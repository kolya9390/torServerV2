package startup

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
	"testing"

	"server/settings"
)

type fakeListener struct{}

func (f fakeListener) Accept() (net.Conn, error) { return nil, errors.New("not implemented") }
func (f fakeListener) Close() error              { return nil }
func (f fakeListener) Addr() net.Addr            { return &net.TCPAddr{} }

type closeFailListener struct{}

func (f closeFailListener) Accept() (net.Conn, error) { return nil, errors.New("not implemented") }
func (f closeFailListener) Close() error              { return errors.New("close failed") }
func (f closeFailListener) Addr() net.Addr            { return &net.TCPAddr{} }

func TestPrepareNetworkDefaultHTTPPort(t *testing.T) {
	prevListen := listenTCP
	listenTCP = func(network, address string) (net.Listener, error) {
		return fakeListener{}, nil
	}

	t.Cleanup(func() {
		listenTCP = prevListen
	})

	args := &settings.ExecArgs{IP: "127.0.0.1", Port: ""}
	if err := PrepareNetworkWithProvider(args, settings.NewNoopSettingsProvider()); err != nil {
		t.Fatalf("PrepareNetwork returned error: %v", err)
	}

	if args.Port != defaultHTTPPort {
		t.Fatalf("expected default http port %s, got %s", defaultHTTPPort, args.Port)
	}
}

func TestPrepareNetworkDetectsBusyHTTPPort(t *testing.T) {
	prevListen := listenTCP
	listenTCP = func(network, address string) (net.Listener, error) {
		return nil, fmt.Errorf("listen tcp %s: %w", address, syscall.EADDRINUSE)
	}

	t.Cleanup(func() {
		listenTCP = prevListen
	})

	args := &settings.ExecArgs{IP: "127.0.0.1", Port: "18090"}
	err := PrepareNetworkWithProvider(args, settings.NewNoopSettingsProvider())
	if err == nil {
		t.Fatal("expected error for busy http port")
	}

	if !strings.Contains(err.Error(), "http port 18090 already in use") {
		t.Fatalf("expected port-in-use error, got %v", err)
	}
}

func TestPrepareNetworkReportsUnderlyingProbeError(t *testing.T) {
	prevListen := listenTCP
	listenTCP = func(network, address string) (net.Listener, error) {
		return nil, errors.New("operation not permitted")
	}

	t.Cleanup(func() {
		listenTCP = prevListen
	})

	args := &settings.ExecArgs{IP: "127.0.0.1", Port: "18090"}
	err := PrepareNetworkWithProvider(args, settings.NewNoopSettingsProvider())
	if err == nil {
		t.Fatal("expected probe error")
	}

	if strings.Contains(err.Error(), "already in use") {
		t.Fatalf("generic probe error must not be masked as port-in-use: %v", err)
	}

	if !strings.Contains(err.Error(), "probe http listener") ||
		!strings.Contains(err.Error(), "operation not permitted") {
		t.Fatalf("expected contextual underlying probe error, got %v", err)
	}
}

func TestPrepareNetworkReportsProbeCloseError(t *testing.T) {
	prevListen := listenTCP
	listenTCP = func(network, address string) (net.Listener, error) {
		return closeFailListener{}, nil
	}

	t.Cleanup(func() {
		listenTCP = prevListen
	})

	args := &settings.ExecArgs{IP: "127.0.0.1", Port: "18090"}
	err := PrepareNetworkWithProvider(args, settings.NewNoopSettingsProvider())
	if err == nil {
		t.Fatal("expected probe close error")
	}

	if !strings.Contains(err.Error(), "close http port 18090 probe listener") ||
		!strings.Contains(err.Error(), "close failed") {
		t.Fatalf("expected contextual close error, got %v", err)
	}
}

func TestPrepareNetworkResolvesSSLPortFromSettings(t *testing.T) {
	prevListen := listenTCP
	listenTCP = func(network, address string) (net.Listener, error) {
		return fakeListener{}, nil
	}

	t.Cleanup(func() {
		listenTCP = prevListen
	})

	args := &settings.ExecArgs{IP: "127.0.0.1", Port: "18090", Ssl: true}
	provider := staticStartupSettingsProvider{cfg: &settings.BTSets{SslPort: 18443}}
	if err := PrepareNetworkWithProvider(args, provider); err != nil {
		t.Fatalf("PrepareNetwork returned error: %v", err)
	}

	if args.SslPort != "18443" {
		t.Fatalf("expected ssl port from settings 18443, got %s", args.SslPort)
	}
}

func TestPrepareNetworkUsesDefaultSSLPortWhenSettingsEmpty(t *testing.T) {
	prevListen := listenTCP
	listenTCP = func(network, address string) (net.Listener, error) {
		return fakeListener{}, nil
	}

	t.Cleanup(func() {
		listenTCP = prevListen
	})

	args := &settings.ExecArgs{IP: "127.0.0.1", Port: "18090", Ssl: true}
	provider := staticStartupSettingsProvider{cfg: &settings.BTSets{SslPort: 0}}
	if err := PrepareNetworkWithProvider(args, provider); err != nil {
		t.Fatalf("PrepareNetwork returned error: %v", err)
	}

	if args.SslPort != defaultHTTPSPort {
		t.Fatalf("expected default ssl port %s, got %s", defaultHTTPSPort, args.SslPort)
	}
}

type staticStartupSettingsProvider struct {
	cfg *settings.BTSets
}

func (p staticStartupSettingsProvider) Get() *settings.BTSets {
	return p.cfg
}

func (staticStartupSettingsProvider) Set(*settings.BTSets) {}

func (staticStartupSettingsProvider) ReadOnly() bool {
	return true
}

func (staticStartupSettingsProvider) GetStaticConfig() settings.StaticConfig {
	return settings.StaticConfig{}
}

func (staticStartupSettingsProvider) GetStoragePreferences() map[string]any {
	return map[string]any{}
}

func (staticStartupSettingsProvider) SetStoragePreferences(map[string]any) error {
	return nil
}
