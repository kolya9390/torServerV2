package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"server/internal/app/contracts"
	"server/settings"
)

type testSettingsProvider struct {
	sets *settings.BTSets
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
