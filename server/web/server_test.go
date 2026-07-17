package web

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"server/internal/app/contracts"
	"server/settings"
	buildversion "server/version"
)

type testSettingsProvider struct {
	sets *settings.BTSets
}

func TestServerStartupMessageUsesInjectedBuildInfo(t *testing.T) {
	t.Parallel()

	info := buildversion.Info{
		Version:   "v1.0.0-rc.1",
		Commit:    "0123456789abcdef",
		BuildTime: "2026-07-16T20:00:00Z",
		Dirty:     buildversion.DirtyClean,
		GoVersion: "go1.26.3",
		OS:        "linux",
		Arch:      "amd64",
		TorrentEngine: buildversion.ModuleInfo{
			Path:    "github.com/anacrolix/torrent",
			Version: "v1.61.0",
		},
	}

	server := NewServerWithDeps(ServerDeps{BuildInfo: info})
	if got, want := server.startupMessage(), buildversion.StartupSummary(info); got != want {
		t.Fatalf("startup message = %q, want %q", got, want)
	}

	if strings.Contains(server.startupMessage(), "2.0.0") {
		t.Fatalf("startup message contains legacy hard-coded version: %q", server.startupMessage())
	}
}

func (p testSettingsProvider) Get() *settings.BTSets {
	if p.sets == nil {
		return &settings.BTSets{}
	}

	cp := *p.sets

	return &cp
}

func (p testSettingsProvider) Set(*settings.BTSets) {}

func (p testSettingsProvider) ReadOnly() bool {
	return false
}

func (p testSettingsProvider) GetStaticConfig() settings.StaticConfig {
	return settings.StaticConfig{}
}

func (p testSettingsProvider) GetStoragePreferences() map[string]any {
	return map[string]any{}
}

func (p testSettingsProvider) SetStoragePreferences(map[string]any) error {
	return nil
}

func TestNewHTTPServerHasHeaderLimitsAndStreamingTimeouts(t *testing.T) {
	srv := newHTTPServer("127.0.0.1:0", http.NewServeMux())

	if srv.ReadHeaderTimeout != httpReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %s, want %s", srv.ReadHeaderTimeout, httpReadHeaderTimeout)
	}

	if srv.MaxHeaderBytes != httpMaxHeaderBytes {
		t.Fatalf("MaxHeaderBytes = %d, want %d", srv.MaxHeaderBytes, httpMaxHeaderBytes)
	}

	if srv.ReadTimeout != 0 {
		t.Fatalf("ReadTimeout = %s, want 0 for streaming", srv.ReadTimeout)
	}

	if srv.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %s, want 0 for streaming", srv.WriteTimeout)
	}

	if srv.IdleTimeout != httpIdleTimeout {
		t.Fatalf("IdleTimeout = %s, want %s", srv.IdleTimeout, httpIdleTimeout)
	}
}

func TestShutdownHTTPServerWithTimeoutClosesActiveStreamingConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("listen unavailable in this environment: %v", err)
	}

	started := make(chan struct{})
	requestDone := make(chan struct{})

	var startedOnce atomic.Bool

	srv := newHTTPServer(listener.Addr().String(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if startedOnce.CompareAndSwap(false, true) {
			close(started)
		}

		w.WriteHeader(http.StatusOK)

		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}

		<-r.Context().Done()
		close(requestDone)
	}))

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- srv.Serve(listener)
	}()

	responseDone := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + listener.Addr().String())
		if err != nil {
			responseDone <- err

			return
		}

		_, err = io.Copy(io.Discard, resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = closeErr
		}

		responseDone <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}

	done := make(chan struct{})
	go func() {
		shutdownHTTPServerWithTimeout("HTTP", srv, 25*time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish after closing active connections")
	}

	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("active request context was not cancelled")
	}

	select {
	case err := <-serveDone:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("Serve() error = %v, want ErrServerClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server loop did not exit")
	}

	select {
	case <-responseDone:
	case <-time.After(time.Second):
		t.Fatal("client response did not finish after server close")
	}
}

func TestRegisterAppRoutesReturnsAPIServicesError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	srv := NewServerWithDeps(ServerDeps{})
	err := srv.registerAppRoutes(gin.New())
	if err == nil {
		t.Fatal("expected API services registration error")
	}

	if !strings.Contains(err.Error(), "register api routes") || !strings.Contains(err.Error(), "services is nil") {
		t.Fatalf("expected contextual API services error, got %v", err)
	}
}

func TestNewServerWithDepsStoresAPIServices(t *testing.T) {
	services := &contracts.APIServices{}
	srv := NewServerWithDeps(ServerDeps{APIServices: services})

	if srv.apiSvc != services {
		t.Fatal("expected APIServices to be assigned during construction")
	}
}

func TestDebugRoutesNotMountedWhenDebugDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	srv := NewServerWithDeps(ServerDeps{
		SettingsProvider: testSettingsProvider{sets: &settings.BTSets{EnableDebug: false, ServiceOnlyDebug: true}},
	})
	route := gin.New()
	srv.registerDebugRoutes(route)

	for _, path := range []string{
		"/debug/vars",
		"/debug/pprof/",
		"/debug/heap",
		"/debug/goroutines",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		route.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected %s to be unmounted with debug disabled, got %d", path, w.Code)
		}
	}
}

func TestDebugRoutesMountedWhenFullDebugEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	srv := NewServerWithDeps(ServerDeps{
		SettingsProvider: testSettingsProvider{sets: &settings.BTSets{EnableDebug: true}},
	})
	route := gin.New()
	srv.registerDebugRoutes(route)

	for _, path := range []string{
		"/debug/vars",
		"/debug/pprof/",
		"/debug/heap",
		"/debug/goroutines",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		route.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound {
			t.Fatalf("expected %s to be mounted with full debug enabled", path)
		}
	}
}
