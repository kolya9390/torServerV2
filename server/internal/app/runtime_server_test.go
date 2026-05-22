package app

import (
	"errors"
	"strings"
	"testing"

	"server/config"
	"server/internal/app/contracts"
	"server/settings"
)

type fakeWebRuntime struct {
	startErr error
	waitErr  error
	started  bool
	stopped  bool
	waited   bool
}

type fakeAPIWebRuntime struct {
	fakeWebRuntime
	apiServices *contracts.APIServices
}

func (f *fakeAPIWebRuntime) SetAPIServices(services *contracts.APIServices) {
	f.apiServices = services
}

func (f *fakeWebRuntime) Start() error {
	f.started = true

	return f.startErr
}

func (f *fakeWebRuntime) Wait() error {
	f.waited = true

	return f.waitErr
}

func (f *fakeWebRuntime) Stop() {
	f.stopped = true
}

func TestServerRuntimeStartRequiresArgs(t *testing.T) {
	restoreArgs := settings.ReplaceArgsForTests(nil)
	t.Cleanup(restoreArgs)

	rt := newServerRuntime(serverRuntimeDeps{}, nil)

	err := rt.Start()
	if err == nil || err.Error() != "exec args are not initialized" {
		t.Fatalf("expected nil-args error, got %v", err)
	}
}

func TestServerRuntimeStartReturnsIncompleteDefaultAPIServicesErrorForAPIWeb(t *testing.T) {
	apiWeb := &fakeAPIWebRuntime{}
	deps := serverRuntimeDeps{
		newWebServer: func() webRuntime { return apiWeb },
	}

	rt := newServerRuntime(deps, nil)
	err := rt.Start()
	if err == nil {
		t.Fatal("expected incomplete API service wiring error")
	}

	if !strings.Contains(err.Error(), "TorrentBackend") {
		t.Fatalf("expected missing TorrentBackend error, got %q", err.Error())
	}

	if apiWeb.apiServices != nil {
		t.Fatalf("api services = %v, want nil", apiWeb.apiServices)
	}
}

func TestServerRuntimeStartPropagatesInitError(t *testing.T) {
	restoreArgs := settings.ReplaceArgsForTests(&settings.ExecArgs{})
	t.Cleanup(restoreArgs)

	initErr := errors.New("init failed")
	deps := serverRuntimeDeps{
		argsProvider: settings.DefaultArgsProvider,
		initSettings: func(readOnly, searchWA bool) error { return initErr },
		setShutdown:  func(func()) {},
	}
	rt := newServerRuntime(deps, nil)

	err := rt.Start()
	if !errors.Is(err, initErr) {
		t.Fatalf("expected init error, got %v", err)
	}
}

func TestServerRuntimeStartPropagatesPrepareError(t *testing.T) {
	restoreArgs := settings.ReplaceArgsForTests(&settings.ExecArgs{})
	t.Cleanup(restoreArgs)

	prepareErr := errors.New("prepare failed")
	deps := serverRuntimeDeps{
		argsProvider:   settings.DefaultArgsProvider,
		initSettings:   func(readOnly, searchWA bool) error { return nil },
		prepareStartup: func(_ *settings.ExecArgs, _ settings.SettingsProvider) error { return prepareErr },
		setShutdown:    func(func()) {},
	}
	rt := newServerRuntime(deps, nil)

	err := rt.Start()
	if !errors.Is(err, prepareErr) {
		t.Fatalf("expected prepare error, got %v", err)
	}
}

func TestServerRuntimeStartAppliesRuntimeSettingsAndPropagatesWebStartError(t *testing.T) {
	restore := settings.ReplaceSettingsForTests(&settings.BTSets{})
	args := &settings.ExecArgs{
		Port:     "18090",
		Ssl:      true,
		SslPort:  "18443",
		SslCert:  "cert.pem",
		SslKey:   "key.pem",
		IP:       "127.0.0.1",
		HTTPAuth: true,
	}
	restoreArgs := settings.ReplaceArgsForTests(args)

	t.Cleanup(func() {
		restoreArgs()
		restore()
	})

	webErr := errors.New("web start failed")
	web := &fakeWebRuntime{startErr: webErr}

	var runtime settings.RuntimeState

	shutdownHookSet := false

	deps := serverRuntimeDeps{
		argsProvider:   settings.DefaultArgsProvider,
		settingsSource: settings.DefaultSettingsProvider,
		runtimeState:   func() settings.RuntimeState { return runtime },
		updateRuntime: func(update func(*settings.RuntimeState)) {
			update(&runtime)
		},
		initSettings:   func(readOnly, searchWA bool) error { return nil },
		prepareStartup: func(_ *settings.ExecArgs, _ settings.SettingsProvider) error { return nil },
		newWebServer:   func() webRuntime { return web },
		setShutdown: func(fn func()) {
			shutdownHookSet = fn != nil
		},
	}
	rt := newServerRuntime(deps, nil)

	err := rt.Start()
	if !errors.Is(err, webErr) {
		t.Fatalf("expected web start error, got %v", err)
	}

	if !shutdownHookSet {
		t.Fatal("expected shutdown hook to be set")
	}

	if runtime.Port != "18090" || runtime.SslPort != "18443" || runtime.IP != "127.0.0.1" {
		t.Fatalf("runtime settings were not applied: port=%s ssl=%s ip=%s", runtime.Port, runtime.SslPort, runtime.IP)
	}

	if !runtime.HTTPAuth {
		t.Fatal("expected HttpAuth to be enabled from args")
	}

	curSets := settings.DefaultSettingsProvider.Get()
	if curSets.SslCert != "cert.pem" || curSets.SslKey != "key.pem" {
		t.Fatalf("expected ssl cert/key to be applied, got cert=%q key=%q", curSets.SslCert, curSets.SslKey)
	}

	if !web.started {
		t.Fatal("expected web start to be called")
	}
}

func TestServerRuntimeStartAppliesConfigToArgsBeforeStartup(t *testing.T) {
	restoreArgs := settings.ReplaceArgsForTests(&settings.ExecArgs{})
	restoreSettings := settings.ReplaceSettingsForTests(&settings.BTSets{})

	t.Cleanup(func() {
		restoreArgs()
		restoreSettings()
	})

	webErr := errors.New("stop before binding test server")
	web := &fakeWebRuntime{startErr: webErr}

	var runtime settings.RuntimeState

	prepareSawConfigPort := false

	deps := serverRuntimeDeps{
		argsProvider:   settings.DefaultArgsProvider,
		settingsSource: settings.DefaultSettingsProvider,
		runtimeState:   func() settings.RuntimeState { return runtime },
		updateRuntime: func(update func(*settings.RuntimeState)) {
			update(&runtime)
		},
		initSettings: func(readOnly, searchWA bool) error { return nil },
		prepareStartup: func(args *settings.ExecArgs, _ settings.SettingsProvider) error {
			prepareSawConfigPort = args.Port == "19090" &&
				args.SslPort == "19443" &&
				args.HTTPAuth &&
				args.SearchWA

			return nil
		},
		newWebServer: func() webRuntime { return web },
		setShutdown:  func(func()) {},
	}
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:     "19090",
			SSLPort:  "19443",
			HTTPAuth: true,
			SearchWA: true,
		},
	}
	rt := newServerRuntime(deps, cfg)

	err := rt.Start()
	if !errors.Is(err, webErr) {
		t.Fatalf("expected web start error, got %v", err)
	}

	if !prepareSawConfigPort {
		t.Fatal("expected config server values to be applied before prepare startup")
	}

	if runtime.Port != "19090" || runtime.SslPort != "19443" {
		t.Fatalf("runtime state did not receive config ports: port=%q ssl_port=%q", runtime.Port, runtime.SslPort)
	}

	if !runtime.HTTPAuth || !runtime.SearchWA {
		t.Fatalf("runtime auth flags not applied: http_auth=%v search_wa=%v", runtime.HTTPAuth, runtime.SearchWA)
	}
}

func TestServerRuntimeWaitAndStop(t *testing.T) {
	waitErr := errors.New("wait failed")
	web := &fakeWebRuntime{waitErr: waitErr}
	closedDB := false

	deps := serverRuntimeDeps{
		newWebServer:  func() webRuntime { return web },
		closeSettings: func() { closedDB = true },
	}
	rt := newServerRuntime(deps, nil)

	if err := rt.Wait(); !errors.Is(err, waitErr) {
		t.Fatalf("expected wait error, got %v", err)
	}

	rt.Stop()

	if !web.stopped || !closedDB {
		t.Fatalf("expected stop chain to be called, web=%v db=%v", web.stopped, closedDB)
	}
}
