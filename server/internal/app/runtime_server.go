package app

import (
	"errors"
	"server/config"
	"server/internal/app/apiservices"
	"server/internal/startup"
	"server/log"
	"server/settings"
	"server/web"
	"server/web/api"
)

type webRuntime interface {
	Start() error
	Wait() error
	Stop()
}

type serverRuntimeDeps struct {
	initSettings   func(readOnly, searchWA bool) error
	prepareStartup func(args *settings.ExecArgs) error
	newWebServer   func() webRuntime
	closeSettings  func()
	setShutdown    func(func())
	setAPIServices func(*api.APIServices)
}

var defaultServerRuntimeDeps = serverRuntimeDeps{
	initSettings:   settings.InitSets,
	prepareStartup: startup.PrepareNetwork,
	newWebServer: func() webRuntime {
		return web.NewServer()
	},
	closeSettings:  settings.CloseDB,
	setShutdown:    nil,
	setAPIServices: api.SetServices,
}

type serverRuntime struct {
	deps        serverRuntimeDeps
	web         webRuntime
	cfg         *config.Config
	apiServices *api.APIServices
}

// NewServerRuntime creates default runtime adapter over startup/settings/web layers.
func NewServerRuntime() Runtime {
	return newServerRuntime(defaultServerRuntimeDeps, nil)
}

func (r *serverRuntime) Start() error {
	args := settings.GetArgs()
	if args == nil {
		return errors.New("exec args are not initialized")
	}

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

	if err := r.deps.prepareStartup(args); err != nil {
		return err
	}

	settings.Ssl = args.Ssl
	if args.Ssl && args.SslCert != "" && args.SslKey != "" {
		settings.BTsets.SslCert = args.SslCert
		settings.BTsets.SslKey = args.SslKey
	}

	log.TLogln("Check web ssl port", args.SslPort)
	log.TLogln("Check web port", args.Port)

	settings.Port = args.Port
	settings.SslPort = args.SslPort
	settings.IP = args.IP
	settings.HttpAuth = args.HttpAuth

	if r.cfg != nil {
		r.applyConfig()
	}

	if err := r.web.Start(); err != nil {
		return err
	}

	return nil
}

func (r *serverRuntime) applyConfig() {
	if r.cfg == nil || settings.BTsets == nil {
		return
	}

	r.cfg.ApplyToBTSets(settings.BTsets)
	settings.SetBTSets(settings.BTsets)
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
	if deps.initSettings == nil {
		deps.initSettings = settings.InitSets
	}

	if deps.prepareStartup == nil {
		deps.prepareStartup = startup.PrepareNetwork
	}

	if deps.newWebServer == nil {
		deps.newWebServer = func() webRuntime { return web.NewServer() }
	}

	if deps.closeSettings == nil {
		deps.closeSettings = settings.CloseDB
	}

	if false {
		// removed
	}

	if deps.setAPIServices == nil {
		deps.setAPIServices = api.SetServices
	}

	runtimeWeb := deps.newWebServer()
	if runtimeWeb == nil {
		runtimeWeb = web.NewServer()
	}

	apiServices := apiservices.NewDefault()

	return &serverRuntime{
		deps:        deps,
		web:         runtimeWeb,
		cfg:         cfg,
		apiServices: apiServices,
	}
}
