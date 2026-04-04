package startup

import (
	"errors"
	"net"
	"testing"

	"server/settings"
)

type fakeListener struct{}

func (f fakeListener) Accept() (net.Conn, error) { return nil, errors.New("not implemented") }
func (f fakeListener) Close() error              { return nil }
func (f fakeListener) Addr() net.Addr            { return &net.TCPAddr{} }

func TestPrepareNetworkDefaultHTTPPort(t *testing.T) {
	prevBT := settings.BTsets
	prevListen := listenTCP
	settings.BTsets = &settings.BTSets{}
	listenTCP = func(network, address string) (net.Listener, error) {
		return fakeListener{}, nil
	}
	t.Cleanup(func() {
		settings.BTsets = prevBT
		listenTCP = prevListen
	})

	args := &settings.ExecArgs{IP: "127.0.0.1", Port: ""}
	if err := PrepareNetwork(args); err != nil {
		t.Fatalf("PrepareNetwork returned error: %v", err)
	}
	if args.Port != defaultHTTPPort {
		t.Fatalf("expected default http port %s, got %s", defaultHTTPPort, args.Port)
	}
}

func TestPrepareNetworkDetectsBusyHTTPPort(t *testing.T) {
	prevBT := settings.BTsets
	prevListen := listenTCP
	settings.BTsets = &settings.BTSets{}
	listenTCP = func(network, address string) (net.Listener, error) {
		return nil, errors.New("busy")
	}
	t.Cleanup(func() {
		settings.BTsets = prevBT
		listenTCP = prevListen
	})

	args := &settings.ExecArgs{IP: "127.0.0.1", Port: "18090"}
	if err := PrepareNetwork(args); err == nil {
		t.Fatal("expected error for busy http port")
	}
}

func TestPrepareNetworkResolvesSSLPortFromSettings(t *testing.T) {
	prevBT := settings.BTsets
	prevListen := listenTCP
	settings.BTsets = &settings.BTSets{SslPort: 18443}
	listenTCP = func(network, address string) (net.Listener, error) {
		return fakeListener{}, nil
	}
	t.Cleanup(func() {
		settings.BTsets = prevBT
		listenTCP = prevListen
	})

	args := &settings.ExecArgs{IP: "127.0.0.1", Port: "18090", Ssl: true}
	if err := PrepareNetwork(args); err != nil {
		t.Fatalf("PrepareNetwork returned error: %v", err)
	}
	if args.SslPort != "18443" {
		t.Fatalf("expected ssl port from settings 18443, got %s", args.SslPort)
	}
}

func TestPrepareNetworkUsesDefaultSSLPortWhenSettingsEmpty(t *testing.T) {
	prevBT := settings.BTsets
	prevListen := listenTCP
	settings.BTsets = &settings.BTSets{SslPort: 0}
	listenTCP = func(network, address string) (net.Listener, error) {
		return fakeListener{}, nil
	}
	t.Cleanup(func() {
		settings.BTsets = prevBT
		listenTCP = prevListen
	})

	args := &settings.ExecArgs{IP: "127.0.0.1", Port: "18090", Ssl: true}
	if err := PrepareNetwork(args); err != nil {
		t.Fatalf("PrepareNetwork returned error: %v", err)
	}
	if args.SslPort != defaultHTTPSPort {
		t.Fatalf("expected default ssl port %s, got %s", defaultHTTPSPort, args.SslPort)
	}
}
