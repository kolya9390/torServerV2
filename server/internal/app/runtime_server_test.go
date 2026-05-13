package app

import (
	"errors"
	"testing"

	"server/settings"
)

type fakeWebRuntime struct {
	startErr error
	waitErr  error
	started  bool
	stopped  bool
	waited   bool
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
	prevArgs := settings.GetArgs()
	settings.Args = nil

	t.Cleanup(func() {
		if prevArgs != nil {
			settings.SetArgs(prevArgs)
		}
	})

	rt := newServerRuntime(serverRuntimeDeps{}, nil)

	err := rt.Start()
	if err == nil || err.Error() != "exec args are not initialized" {
		t.Fatalf("expected nil-args error, got %v", err)
	}
}

func TestServerRuntimeStartPropagatesInitError(t *testing.T) {
	prevArgs := settings.GetArgs()
	settings.SetArgs(&settings.ExecArgs{})
	t.Cleanup(func() {
		settings.SetArgs(prevArgs)
	})

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
	prevArgs := settings.GetArgs()
	settings.SetArgs(&settings.ExecArgs{})
	t.Cleanup(func() {
		settings.SetArgs(prevArgs)
	})

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
	prevArgs := settings.GetArgs()
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
	settings.SetArgs(args)
	t.Cleanup(func() {
		settings.SetArgs(prevArgs)
		restore()
	})

	webErr := errors.New("web start failed")
	web := &fakeWebRuntime{startErr: webErr}
	shutdownHookSet := false
	deps := serverRuntimeDeps{
		argsProvider:   settings.DefaultArgsProvider,
		settingsSource: settings.DefaultSettingsProvider,
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

	runtime := settings.GetRuntimeState()
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
