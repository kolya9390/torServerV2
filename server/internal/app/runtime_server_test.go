package app

import (
	"errors"
	"testing"

	"server/config"
	"server/internal/app/contracts"
	"server/settings"
	"server/torr"
	"server/web"
)

type fakeWebRuntime struct {
	startErr error
	waitErr  error
	started  bool
	stopped  bool
	waited   bool
}

type runtimeTestSettingsProvider struct {
	sets *settings.BTSets
}

func (p *runtimeTestSettingsProvider) Get() *settings.BTSets {
	if p.sets == nil {
		return &settings.BTSets{}
	}

	cp := *p.sets

	return &cp
}

func (p *runtimeTestSettingsProvider) Set(sets *settings.BTSets) {
	if sets == nil {
		p.sets = nil

		return
	}

	cp := *sets
	p.sets = &cp
}

func (p *runtimeTestSettingsProvider) ReadOnly() bool {
	return false
}

func (p *runtimeTestSettingsProvider) GetStaticConfig() settings.StaticConfig {
	return settings.StaticConfig{}
}

func (p *runtimeTestSettingsProvider) GetStoragePreferences() map[string]any {
	return map[string]any{}
}

func (p *runtimeTestSettingsProvider) SetStoragePreferences(map[string]any) error {
	return nil
}

type runtimeTestArgsProvider struct {
	args *settings.ExecArgs
}

func (p runtimeTestArgsProvider) Get() *settings.ExecArgs {
	if p.args == nil {
		return nil
	}

	cp := *p.args

	return &cp
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

func TestServerRuntimeStartReturnsAPIServicesFactoryError(t *testing.T) {
	apiErr := errors.New("api services failed")
	fakeWeb := &fakeWebRuntime{}
	deps := serverRuntimeDeps{
		newWebServer: func(web.ServerDeps) webRuntime { return fakeWeb },
		newAPIServices: func(*torr.BTServer) (*contracts.APIServices, error) {
			return nil, apiErr
		},
	}

	rt := newServerRuntime(deps, nil)
	err := rt.Start()
	if !errors.Is(err, apiErr) {
		t.Fatalf("expected API services error, got %v", err)
	}

	if fakeWeb.started {
		t.Fatal("expected web runtime not to start when API services factory fails")
	}
}

func TestServerRuntimePassesAPIServicesThroughWebServerDeps(t *testing.T) {
	apiServices := &contracts.APIServices{}

	var captured web.ServerDeps

	deps := serverRuntimeDeps{
		newAPIServices: func(bt *torr.BTServer) (*contracts.APIServices, error) {
			if bt == nil {
				t.Fatal("expected BTServer to be built before APIServices")
			}

			return apiServices, nil
		},
		newWebServer: func(deps web.ServerDeps) webRuntime {
			captured = deps

			return &fakeWebRuntime{}
		},
	}

	rt := newServerRuntime(deps, nil)
	serverRt, ok := rt.(*serverRuntime)
	if !ok {
		t.Fatalf("runtime type = %T, want *serverRuntime", rt)
	}

	if serverRt.APIServices() != apiServices {
		t.Fatal("expected runtime to retain constructed APIServices")
	}

	if captured.APIServices != apiServices {
		t.Fatal("expected APIServices to be passed through web.ServerDeps")
	}

	if captured.BTServer == nil {
		t.Fatal("expected BTServer to be passed through web.ServerDeps")
	}
}

func TestServerRuntimeDefaultAPIServicesUseInjectedProviders(t *testing.T) {
	provider := &runtimeTestSettingsProvider{
		sets: &settings.BTSets{
			EnableDebug: true,
		},
	}
	argsProvider := runtimeTestArgsProvider{args: &settings.ExecArgs{Port: "18090"}}
	runtimeState := settings.RuntimeState{Port: "18090"}

	var captured web.ServerDeps

	deps := serverRuntimeDeps{
		argsProvider:   argsProvider,
		settingsSource: provider,
		runtimeState:   func() settings.RuntimeState { return runtimeState },
		newWebServer: func(deps web.ServerDeps) webRuntime {
			captured = deps

			return &fakeWebRuntime{}
		},
	}

	rt := newServerRuntime(deps, nil)
	serverRt, ok := rt.(*serverRuntime)
	if !ok {
		t.Fatalf("runtime type = %T, want *serverRuntime", rt)
	}

	if serverRt.servicesErr != nil {
		t.Fatalf("expected default API services to be built, got %v", serverRt.servicesErr)
	}

	if captured.APIServices == nil || captured.APIServices.Settings == nil {
		t.Fatal("expected API services to be passed through web deps")
	}

	current := captured.APIServices.Settings.Current()
	if current == nil || !current.EnableDebug {
		t.Fatalf("expected API settings service to use injected provider, got %#v", current)
	}

	if captured.ArgsProvider != argsProvider {
		t.Fatal("expected web deps to retain injected args provider")
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
	fakeWeb := &fakeWebRuntime{startErr: webErr}

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
		newWebServer:   func(web.ServerDeps) webRuntime { return fakeWeb },
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

	if !fakeWeb.started {
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
	fakeWeb := &fakeWebRuntime{startErr: webErr}

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
		newWebServer: func(web.ServerDeps) webRuntime { return fakeWeb },
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
	fakeWeb := &fakeWebRuntime{waitErr: waitErr}
	closeDBCalls := 0

	deps := serverRuntimeDeps{
		newWebServer:  func(web.ServerDeps) webRuntime { return fakeWeb },
		closeSettings: func() { closeDBCalls++ },
	}
	rt := newServerRuntime(deps, nil)

	if err := rt.Wait(); !errors.Is(err, waitErr) {
		t.Fatalf("expected wait error, got %v", err)
	}

	rt.Stop()

	if !fakeWeb.stopped || closeDBCalls != 1 {
		t.Fatalf("expected stop chain to be called, web=%v db_calls=%d", fakeWeb.stopped, closeDBCalls)
	}
}

func TestServerRuntimeStopIsIdempotent(t *testing.T) {
	fakeWeb := &fakeWebRuntime{}
	closeDBCalls := 0

	deps := serverRuntimeDeps{
		newWebServer:  func(web.ServerDeps) webRuntime { return fakeWeb },
		closeSettings: func() { closeDBCalls++ },
	}
	rt := newServerRuntime(deps, nil)

	rt.Stop()
	rt.Stop()

	if !fakeWeb.stopped {
		t.Fatal("expected web runtime to be stopped")
	}

	if closeDBCalls != 1 {
		t.Fatalf("expected close settings to be called once, got %d", closeDBCalls)
	}
}
