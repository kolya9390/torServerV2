package web

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"net/http"
	"net/http/pprof"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/location/v2"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"server/dlna"
	"server/log"
	"server/metrics"
	"server/modules"
	"server/settings"
	"server/torr"
	"server/torrfs"
	"server/torrfs/webdav"
	"server/web/api"
	"server/web/auth"
	"server/web/blocker"
	"server/web/webinfra"
)

type ServerDeps struct {
	BTServer         *torr.BTServer
	TorrentDBStore   torr.TorrentDBStore
	SettingsProvider settings.SettingsProvider
	ArgsProvider     settings.ArgsProvider
	RuntimeState     func() settings.RuntimeState
	CORSService      webinfra.CORSService
	SSLService       webinfra.SSLService
}

type Server struct {
	bts        *torr.BTServer
	waitChan   chan error
	mu         sync.RWMutex
	httpServer *http.Server
	httpsSrv   *http.Server
	corsSvc    webinfra.CORSService
	sslSvc     webinfra.SSLService
	settings   settings.SettingsProvider
	args       settings.ArgsProvider
	runtime    func() settings.RuntimeState
}

func NewServerWithDeps(deps ServerDeps) *Server {
	if deps.BTServer == nil {
		deps.BTServer = torr.NewBTSWithProvidersRuntimeAndDB(deps.SettingsProvider, deps.ArgsProvider, deps.RuntimeState, deps.TorrentDBStore)
	}
	if deps.RuntimeState == nil {
		deps.RuntimeState = func() settings.RuntimeState { return settings.RuntimeState{} }
	}

	return &Server{
		bts:      deps.BTServer,
		waitChan: make(chan error, 2),
		corsSvc:  deps.CORSService,
		sslSvc:   deps.SSLService,
		settings: deps.SettingsProvider,
		args:     deps.ArgsProvider,
		runtime:  deps.RuntimeState,
	}
}

func (s *Server) currentSettings() *settings.BTSets {
	if s != nil && s.settings != nil {
		return s.settings.Get()
	}

	return nil
}

func (s *Server) currentArgs() *settings.ExecArgs {
	if s != nil && s.args != nil {
		return s.args.Get()
	}

	return nil
}

func (s *Server) ensureInfraServices() {
	if s.corsSvc == nil {
		s.corsSvc = webinfra.NewCORSServiceWithProviders(s.args)
	}

	if s.sslSvc == nil {
		s.sslSvc = webinfra.NewSSLServiceWithProvidersAndRuntime(s.settings, s.args, s.currentRuntimeState)
	}
}

func (s *Server) currentRuntimeState() settings.RuntimeState {
	if s != nil && s.runtime != nil {
		return s.runtime()
	}

	return settings.RuntimeState{}
}

func (s *Server) debugEnabled() bool {
	curSets := s.currentSettings()
	if curSets == nil {
		return false
	}

	return curSets.DebugConfig().EnableDebug
}

func (s *Server) BTServer() *torr.BTServer {
	if s == nil {
		return nil
	}

	return s.bts
}

func (s *Server) Start() error {
	s.ensureInfraServices()

	log.TLogln("Start TorrServer 2.0.0")

	ips := webinfra.GetLocalIps()
	if len(ips) > 0 {
		log.TLogln("Local IPs:", ips)
	}

	if err := s.bts.Connect(); err != nil {
		return fmt.Errorf("BTS.Connect() error: %w", err)
	}

	catalog := torr.NewTorrentServiceWithBT(s.bts)
	dlna.SetCatalog(catalog)
	torrfs.SetCatalog(catalog)

	// Initialize runtime metrics
	metrics.InitWithDeps(metrics.Deps{
		SettingsProvider: s.settings,
		TorrentBackend:   torr.NewTorrentServiceWithBT(s.bts),
	})

	route := setupMiddleware(s)
	s.registerDebugRoutes(route)

	// Swagger UI (accessible at /swagger/index.html)
	route.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	s.registerAppRoutes(route)

	if err := s.startHTTPSServer(route, ips); err != nil {
		return err
	}

	s.startHTTPServer(route)

	return nil
}

// setupMiddleware configures CORS, logging, recovery, security headers, and auth middleware.
func setupMiddleware(s *Server) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	corsCfg := s.corsSvc.BuildConfig()
	route := gin.New()

	if err := route.SetTrustedProxies(webinfra.CheckTrustedProxies()); err != nil {
		log.TLogln("Invalid trusted proxies config:", err)
	}

	route.Use(
		log.RequestIDMiddleware(),
		log.WebLogger(),
		blocker.BlockerWithRuntimeState(s.currentRuntimeState),
		gin.Recovery(),
		cors.New(corsCfg),
		location.Default(),
		securityHeadersMiddleware(),
		api.ErrorResponder(),
	)
	auth.InitAuthWithRuntimeState(s.currentRuntimeState)
	auth.SetupAuth(route)

	return route
}

// registerDebugRoutes registers health check, echo, and pprof/debug endpoints.
// RootHandler returns a simple status for root requests (used by clients like Lampa for detection).
func rootHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"server":  "TorrServer",
		"version": "v1",
		"status":  "ok",
	})
}

func (s *Server) registerDebugRoutes(route *gin.Engine) {
	route.GET("/", rootHandler)
	route.GET("/echo", echo)
	route.GET("/healthz", healthz)
	route.GET("/readyz", s.readyz)

	if !s.debugEnabled() {
		return
	}

	route.GET("/debug/vars", expvarHandler())
	route.GET("/debug/pprof/", pprofIndex())
	route.GET("/debug/pprof/profile", pprofProfile())
	route.GET("/debug/pprof/trace", pprofTrace())
	route.GET("/debug/pprof/cmdline", pprofCmdline())
	route.GET("/debug/pprof/symbol", pprofSymbol())
	route.GET("/debug/pprof/allocs", pprofAllocs())
	route.GET("/debug/pprof/block", pprofBlock())
	route.GET("/debug/pprof/mutex", pprofMutex())
	route.GET("/debug/pprof/threadcreate", pprofThreadcreate())
	route.GET("/debug/pprof/heap", heapHandler())
	route.GET("/debug/pprof/goroutine", goroutinesHandler())
	route.GET("/debug/heap", heapHandler())
	route.GET("/debug/goroutines", goroutinesHandler())
}

// registerAppRoutes registers API routes and optional WebDAV/DLNA/FUSE modules.
func (s *Server) registerAppRoutes(route *gin.Engine) {
	api.SetupRouteWithRuntimeState(route, s.currentRuntimeState)

	args := s.currentArgs()
	if args != nil && args.WebDAV {
		webdav.MountWebDAV(route)
	}

	dlnaCfg := s.currentSettings().DLNAConfig()
	if dlnaCfg.Enabled {
		modules.LogPeripheralFailure("dlna", modules.RestartDLNAWithProviders(true, s.settings, s.args))
	}

	modules.LogPeripheralFailure("fuse", modules.StartFUSEWithProviders(s.settings, s.args))
}

// startHTTPSServer starts the HTTPS server if SSL is enabled.
func (s *Server) startHTTPSServer(route *gin.Engine, ips []string) error {
	args := s.currentArgs()
	if args == nil || !args.Ssl {
		return nil
	}

	if err := s.sslSvc.PrepareCertificates(ips); err != nil {
		return fmt.Errorf("SSL prepare error: %w", err)
	}

	if err := s.sslSvc.VerifyOrRegenerateCerts(ips); err != nil {
		return fmt.Errorf("SSL verify error: %w", err)
	}

	httpsAddr := args.IP + ":" + args.SslPort
	httpsSrv := s.sslSvc.Server(httpsAddr, route)

	s.mu.Lock()
	s.httpsSrv = httpsSrv
	s.mu.Unlock()

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				s.waitChan <- fmt.Errorf("panic in https server loop: %v", rec)
			}
		}()
		log.TLogln("Start https server at", httpsAddr)

		tlsCfg := s.currentSettings().TLSConfig()
		err := httpsSrv.ListenAndServeTLS(tlsCfg.Cert, tlsCfg.Key)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.waitChan <- err

			return
		}
		s.waitChan <- nil
	}()

	return nil
}

// startHTTPServer starts the HTTP server on the configured address.
func (s *Server) startHTTPServer(route *gin.Engine) {
	args := s.currentArgs()
	if args == nil {
		args = &settings.ExecArgs{}
	}

	httpAddr := args.IP + ":" + args.Port
	httpSrv := &http.Server{
		Addr:         httpAddr,
		Handler:      route,
		ReadTimeout:  0, // No timeout - streaming connections
		WriteTimeout: 0, // No timeout - streaming connections
		IdleTimeout:  60 * time.Second,
	}

	s.mu.Lock()
	s.httpServer = httpSrv
	s.mu.Unlock()

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				s.waitChan <- fmt.Errorf("panic in http server loop: %v", rec)
			}
		}()
		log.TLogln("Start http server at", httpAddr)

		err := httpSrv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.waitChan <- err

			return
		}
		s.waitChan <- nil
	}()
}

func (s *Server) Wait() error {
	err := <-s.waitChan
	if err != nil && errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}

func (s *Server) Stop() {
	s.mu.Lock()
	httpLocal := s.httpServer
	httpsLocal := s.httpsSrv
	s.httpServer = nil
	s.httpsSrv = nil
	s.mu.Unlock()

	log.TLogln("Stopping TorrServer components...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if httpsLocal != nil {
		if err := httpsLocal.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.TLogln("HTTPS shutdown error:", err)
		}
	}

	if httpLocal != nil {
		if err := httpLocal.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.TLogln("HTTP shutdown error:", err)
		}
	}

	modules.StopDLNA()
	modules.StopFUSE()
	s.bts.Disconnect()

	log.TLogln("TorrServer stopped")
}

func echo(c *gin.Context) {
	c.String(200, "1.0.0")
}

func healthz(c *gin.Context) {
	c.String(200, "OK")
}

func (s *Server) readyz(c *gin.Context) {
	if s == nil {
		c.JSON(200, gin.H{
			"status":  "ready",
			"http":    false,
			"torrent": false,
		})

		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	status := gin.H{
		"status":  "ready",
		"http":    s.httpServer != nil,
		"torrent": s.bts != nil,
	}
	c.JSON(200, status)
}

func expvarHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "application/json; charset=utf-8")
		expvar.Handler().ServeHTTP(c.Writer, c.Request)
	}
}

func heapHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		pprof.Handler("heap").ServeHTTP(c.Writer, c.Request)
	}
}

func goroutinesHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		pprof.Handler("goroutine").ServeHTTP(c.Writer, c.Request)
	}
}

// pprof wrapper handlers.
func pprofIndex() gin.HandlerFunc        { return gin.WrapF(pprof.Index) }
func pprofProfile() gin.HandlerFunc      { return gin.WrapF(pprof.Profile) }
func pprofTrace() gin.HandlerFunc        { return gin.WrapF(pprof.Trace) }
func pprofCmdline() gin.HandlerFunc      { return gin.WrapF(pprof.Cmdline) }
func pprofSymbol() gin.HandlerFunc       { return gin.WrapF(pprof.Symbol) }
func pprofAllocs() gin.HandlerFunc       { return gin.WrapF(pprof.Handler("allocs").ServeHTTP) }
func pprofBlock() gin.HandlerFunc        { return gin.WrapF(pprof.Handler("block").ServeHTTP) }
func pprofMutex() gin.HandlerFunc        { return gin.WrapF(pprof.Handler("mutex").ServeHTTP) }
func pprofThreadcreate() gin.HandlerFunc { return gin.WrapF(pprof.Handler("threadcreate").ServeHTTP) }
