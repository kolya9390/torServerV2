package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/location/v2"
	"github.com/gin-gonic/gin"

	"server/log"
	"server/modules"
	"server/settings"
	"server/torr"
	"server/torrfs/webdav"
	"server/web/api"
	"server/web/auth"
	"server/web/blocker"
	"server/web/webinfra"
)

var (
	defaultServerMu sync.Mutex
	defaultServer   = NewServer()
)

type Server struct {
	bts        *torr.BTServer
	waitChan   chan error
	mu         sync.RWMutex
	httpServer *http.Server
	httpsSrv   *http.Server
	corsSvc    webinfra.CORSService
	sslSvc     webinfra.SSLService
}

func NewServer() *Server {
	return &Server{
		bts:      torr.NewBTS(),
		waitChan: make(chan error, 2),
		corsSvc:  webinfra.NewCORSService(),
		sslSvc:   webinfra.NewSSLService(),
	}
}

func getDefaultServer() *Server {
	defaultServerMu.Lock()
	defer defaultServerMu.Unlock()
	if defaultServer == nil {
		defaultServer = NewServer()
	}
	return defaultServer
}

func Start() error {
	return getDefaultServer().Start()
}

func (s *Server) Start() error {
	log.TLogln("Start TorrServer 3.0.0")
	ips := webinfra.GetLocalIps()
	if len(ips) > 0 {
		log.TLogln("Local IPs:", ips)
	}
	err := s.bts.Connect()
	if err != nil {
		return fmt.Errorf("BTS.Connect() error: %w", err)
	}

	gin.SetMode(gin.ReleaseMode)

	corsCfg := s.corsSvc.BuildConfig()

	route := gin.New()
	trustedProxies := webinfra.CheckTrustedProxies()
	if err := route.SetTrustedProxies(trustedProxies); err != nil {
		log.TLogln("Invalid trusted proxies config:", err)
	}

	route.Use(log.RequestIDMiddleware(), log.WebLogger(), blocker.Blocker(), gin.Recovery(), cors.New(corsCfg), location.Default(), securityHeadersMiddleware(), api.ErrorResponder())
	auth.SetupAuth(route)

	route.GET("/echo", echo)
	route.GET("/healthz", healthz)
	route.GET("/readyz", readyz)
	route.GET("/debug/heap", heapHandler())
	route.GET("/debug/goroutines", goroutinesHandler())

	api.SetupRoute(route)
	args := settings.GetArgs()
	if args != nil && args.WebDAV {
		webdav.MountWebDAV(route)
	}

	if settings.BTsets.EnableDLNA {
		modules.LogPeripheralFailure("dlna", modules.RestartDLNA(true))
	}

	modules.LogPeripheralFailure("fuse", modules.StartFUSE())

	if settings.Ssl {
		if err := s.sslSvc.PrepareCertificates(ips); err != nil {
			return fmt.Errorf("SSL prepare error: %w", err)
		}
		if err := s.sslSvc.VerifyOrRegenerateCerts(ips); err != nil {
			return fmt.Errorf("SSL verify error: %w", err)
		}

		httpsAddr := settings.IP + ":" + settings.SslPort
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
			err := httpsSrv.ListenAndServeTLS(settings.BTsets.SslCert, settings.BTsets.SslKey)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.waitChan <- err
				return
			}
			s.waitChan <- nil
		}()
	}

	httpAddr := settings.IP + ":" + settings.Port
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

	return nil
}

func Wait() error {
	return getDefaultServer().Wait()
}

func (s *Server) Wait() error {
	err := <-s.waitChan
	if err != nil && errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func Stop() {
	getDefaultServer().Stop()
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
	c.String(200, "3.0.0")
}

func healthz(c *gin.Context) {
	c.String(200, "OK")
}

func readyz(c *gin.Context) {
	s := getDefaultServer()
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := gin.H{
		"status":  "ready",
		"http":    s.httpServer != nil,
		"torrent": s.bts != nil,
	}
	c.JSON(200, status)
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
