package app

import (
	"errors"
	"os"
	"server/config"
	"server/internal/app/apiservices"
	"server/internal/startup"
	"server/log"
	"server/settings"
	"server/torr"
	"server/web"
	"server/web/api"
	"server/web/auth"
	"strings"
)

type webRuntime interface {
	Start() error
	Wait() error
	Stop()
}

type btServerProvider interface {
	BTServer() *torr.BTServer
}

type serverRuntimeDeps struct {
	argsProvider   settings.ArgsProvider
	settingsSource settings.SettingsProvider
	dbStore        torr.TorrentDBStore
	initSettings   func(readOnly, searchWA bool) error
	prepareStartup func(args *settings.ExecArgs, provider settings.SettingsProvider) error
	newWebServer   func() webRuntime
	newAPIServices func(*torr.BTServer) *api.APIServices
	closeSettings  func()
	setShutdown    func(func())
	setAPIServices func(*api.APIServices)
}

func defaultServerRuntimeDeps() serverRuntimeDeps {
	return serverRuntimeDeps{
		argsProvider:   settings.DefaultArgsProvider,
		settingsSource: settings.DefaultSettingsProvider,
		dbStore:        torr.NewSettingsTorrentDBStore(),
		initSettings:   settings.InitSets,
		prepareStartup: startup.PrepareNetworkWithProvider,
		newWebServer: func() webRuntime {
			return web.NewServerWithDeps(web.ServerDeps{
				TorrentDBStore:   torr.NewSettingsTorrentDBStore(),
				SettingsProvider: settings.DefaultSettingsProvider,
				ArgsProvider:     settings.DefaultArgsProvider,
				RuntimeState:     settings.GetRuntimeState,
			})
		},
		newAPIServices: func(bt *torr.BTServer) *api.APIServices {
			return apiservices.NewDefaultWithDeps(apiservices.DefaultDeps{
				TorrentBackend:    torr.NewTorrentServiceWithBT(bt),
				SettingsProvider:  settings.DefaultSettingsProvider,
				RuntimeSignals:    torr.NewRuntimeSignalsWithBT(bt),
				RuntimeController: torr.NewRuntimeControllerWithBT(bt),
				RuntimeState:      settings.GetRuntimeState,
				ArgsProvider:      settings.DefaultArgsProvider,
				SetViewed:         settings.SetViewed,
				RemoveViewed:      settings.RemViewed,
				ListViewed:        settings.ListViewed,
			})
		},
		closeSettings:  settings.CloseDB,
		setShutdown:    api.SetShutdownHook,
		setAPIServices: api.SetServices,
	}
}

// NewRuntimeWithConfig builds the explicit application runtime with optional config.
func NewRuntimeWithConfig(cfg *config.Config) Runtime {
	return newServerRuntime(defaultServerRuntimeDeps(), cfg)
}

type serverRuntime struct {
	deps        serverRuntimeDeps
	web         webRuntime
	cfg         *config.Config
	apiServices *api.APIServices
}

func (r *serverRuntime) Start() error {
	args := r.currentArgs()
	if args == nil {
		return errors.New("exec args are not initialized")
	}

	r.applyConfigToArgs(args)

	if r.deps.setShutdown != nil {
		r.deps.setShutdown(r.Stop)
	}

	if err := r.deps.initSettings(args.RDB, args.SearchWA); err != nil {
		return err
	}

	if r.deps.setAPIServices != nil {
		r.deps.setAPIServices(r.apiServices)
	} else {
		api.SetServices(r.apiServices)
	}

	if err := r.deps.prepareStartup(args, r.deps.settingsSource); err != nil {
		return err
	}
	settings.SetArgs(args)

	if args.Ssl && args.SslCert != "" && args.SslKey != "" {
		curSets := r.currentSettings()
		curSets.SslCert = args.SslCert
		curSets.SslKey = args.SslKey
	}

	log.TLogln("Check web ssl port", args.SslPort)
	log.TLogln("Check web port", args.Port)

	settings.UpdateRuntimeState(func(runtime *settings.RuntimeState) {
		runtime.IP = args.IP
		runtime.Port = args.Port
		runtime.Ssl = args.Ssl
		runtime.SslPort = args.SslPort
		runtime.HTTPAuth = args.HTTPAuth
		runtime.SearchWA = args.SearchWA
	})

	if r.cfg != nil {
		r.applyConfig()
	}

	mode, token := resolveShutdownConfig(args, r.cfg)
	api.ConfigureShutdown(mode, token)

	if err := r.web.Start(); err != nil {
		return err
	}

	return nil
}

func (r *serverRuntime) applyConfigToArgs(args *settings.ExecArgs) {
	if args == nil || r.cfg == nil {
		return
	}

	if args.Port == "" {
		args.Port = r.cfg.Server.Port
	}

	if !args.Ssl {
		args.Ssl = r.cfg.Server.SSL
	}

	if args.SslPort == "" {
		args.SslPort = r.cfg.Server.SSLPort
	}

	if args.SslCert == "" {
		args.SslCert = r.cfg.Server.SSLCert
	}

	if args.SslKey == "" {
		args.SslKey = r.cfg.Server.SSLKey
	}

	if !args.HTTPAuth {
		args.HTTPAuth = r.cfg.Server.HTTPAuth
	}

	if !args.SearchWA {
		args.SearchWA = r.cfg.Server.SearchWA
	}

	if args.ShutdownMode == "" {
		args.ShutdownMode = r.cfg.Server.ShutdownMode
	}
}

func (r *serverRuntime) applyConfig() {
	if r.cfg == nil {
		return
	}

	curSets := r.currentSettings()
	r.cfg.ApplyToBTSets(curSets)
	r.setCurrentSettings(curSets)
	log.TLogln("Applied configuration from config.yml")
}

func (r *serverRuntime) Stop() {
	r.web.Stop()
	r.deps.closeSettings()
}

func (r *serverRuntime) Wait() error {
	return r.web.Wait()
}

func (r *serverRuntime) APIServices() *api.APIServices {
	return r.apiServices
}

func newServerRuntime(deps serverRuntimeDeps, cfg *config.Config) Runtime {
	if deps.argsProvider == nil {
		deps.argsProvider = settings.NewNoopArgsProvider()
	}

	if deps.settingsSource == nil {
		deps.settingsSource = settings.NewNoopSettingsProvider()
	}

	if deps.initSettings == nil {
		deps.initSettings = settings.InitSets
	}

	if deps.prepareStartup == nil {
		deps.prepareStartup = startup.PrepareNetworkWithProvider
	}

	if deps.dbStore == nil {
		deps.dbStore = torr.NewNoopTorrentDBStore()
	}

	if deps.newWebServer == nil {
		deps.newWebServer = func() webRuntime { return newDefaultWebRuntime(deps) }
	}

	if deps.newAPIServices == nil {
		deps.newAPIServices = func(bt *torr.BTServer) *api.APIServices {
			return newDefaultAPIServices(deps, bt)
		}
	}

	if deps.closeSettings == nil {
		deps.closeSettings = settings.CloseDB
	}

	if deps.setAPIServices == nil {
		deps.setAPIServices = api.SetServices
	}

	runtimeWeb := deps.newWebServer()
	if runtimeWeb == nil {
		runtimeWeb = newDefaultWebRuntime(deps)
	}

	var bt *torr.BTServer
	if provider, ok := runtimeWeb.(btServerProvider); ok {
		bt = provider.BTServer()
	}

	apiServices := deps.newAPIServices(bt)
	if apiServices == nil {
		apiServices = newDefaultAPIServices(deps, bt)
	}

	return &serverRuntime{
		deps:        deps,
		web:         runtimeWeb,
		cfg:         cfg,
		apiServices: apiServices,
	}
}

func newDefaultWebRuntime(deps serverRuntimeDeps) webRuntime {
	return web.NewServerWithDeps(web.ServerDeps{
		TorrentDBStore:   deps.dbStore,
		SettingsProvider: deps.settingsSource,
		ArgsProvider:     deps.argsProvider,
		RuntimeState:     settings.GetRuntimeState,
	})
}

func newDefaultAPIServices(deps serverRuntimeDeps, bt *torr.BTServer) *api.APIServices {
	defaultDeps := apiservices.DefaultDeps{
		SettingsProvider: deps.settingsSource,
		RuntimeState:     settings.GetRuntimeState,
		ArgsProvider:     deps.argsProvider,
		SetViewed:        settings.SetViewed,
		RemoveViewed:     settings.RemViewed,
		ListViewed:       settings.ListViewed,
	}

	if bt != nil {
		defaultDeps.TorrentBackend = torr.NewTorrentServiceWithBT(bt)
		defaultDeps.RuntimeSignals = torr.NewRuntimeSignalsWithBT(bt)
		defaultDeps.RuntimeController = torr.NewRuntimeControllerWithBT(bt)
	}

	return apiservices.NewDefaultWithDeps(defaultDeps)
}

func (r *serverRuntime) currentArgs() *settings.ExecArgs {
	if r != nil && r.deps.argsProvider != nil {
		return r.deps.argsProvider.Get()
	}

	return nil
}

func (r *serverRuntime) currentSettings() *settings.BTSets {
	if r != nil && r.deps.settingsSource != nil {
		return r.deps.settingsSource.Get()
	}

	return nil
}

func (r *serverRuntime) setCurrentSettings(sets *settings.BTSets) {
	if sets == nil {
		return
	}

	if r != nil && r.deps.settingsSource != nil {
		r.deps.settingsSource.Set(sets)
	}
}

func resolveShutdownConfig(args *settings.ExecArgs, cfg *config.Config) (string, string) {
	mode := strings.TrimSpace(args.ShutdownMode)
	if mode == "" {
		mode = strings.TrimSpace(os.Getenv("TS_SHUTDOWN_MODE"))
	}

	if mode == "" && cfg != nil {
		mode = strings.TrimSpace(cfg.Server.ShutdownMode)
	}

	token := strings.TrimSpace(args.ShutdownToken)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("TS_SHUTDOWN_TOKEN"))
	}

	// Check BBolt-stored token (SEC4: secure storage)
	if token == "" {
		if dbToken, err := auth.GetShutdownToken(); err == nil && dbToken != "" {
			token = dbToken
		}
	}

	return mode, token
}
