package api

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	sets "server/settings"
	"server/web/auth"
)

func TestProtectedEndpointsRequireAuth(t *testing.T) {
	r := setupAuthRouter(t, false)

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/settings", body: `{}`},
		{method: http.MethodPost, path: "/api/v1/settings", body: `{}`},
		{method: http.MethodPost, path: "/api/v1/torrents", body: `{}`},
		{method: http.MethodGet, path: "/api/v1/storage/settings"},
		{method: http.MethodPost, path: "/api/v1/streams/save"},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for %s %s, got %d body=%s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}

func TestProtectedEndpointAllowsValidAuth(t *testing.T) {
	r := setupAuthRouter(t, false)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", basicHeader("admin", "secret"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("expected non-401 for authenticated request, got %d", w.Code)
	}
}

func TestNoAuthModeKeepsEndpointsAccessible(t *testing.T) {
	gin.SetMode(gin.TestMode)

	prevHTTPAuth := sets.HttpAuth
	sets.HttpAuth = false

	t.Cleanup(func() {
		sets.HttpAuth = prevHTTPAuth
	})

	r := gin.New()
	SetupRoute(r)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("expected non-401 when HttpAuth=false, got %d", w.Code)
	}
}

func TestTorznabSearchAuthParityBySearchWA(t *testing.T) {
	protectedRouter := setupAuthRouter(t, false)
	protectedReq := httptest.NewRequest(http.MethodGet, "/api/v1/torznab/search/test?query=abc", nil)
	protectedW := httptest.NewRecorder()
	protectedRouter.ServeHTTP(protectedW, protectedReq)

	if protectedW.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when SearchWA=false, got %d body=%s", protectedW.Code, protectedW.Body.String())
	}

	publicRouter := setupAuthRouter(t, true)
	publicReq := httptest.NewRequest(http.MethodGet, "/api/v1/torznab/search/test?query=abc", nil)
	publicW := httptest.NewRecorder()
	publicRouter.ServeHTTP(publicW, publicReq)

	if publicW.Code == http.StatusUnauthorized {
		t.Fatalf("expected non-401 when SearchWA=true, got %d", publicW.Code)
	}
}

func setupAuthRouter(t *testing.T, searchWA bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()

	accsPath := filepath.Join(tmpDir, "accs.db")
	if err := os.WriteFile(accsPath, []byte(`{"admin":"secret"}`), 0o644); err != nil {
		t.Fatalf("write accs.db: %v", err)
	}

	prevPath := sets.Path
	prevHTTPAuth := sets.HttpAuth
	prevSearchWA := sets.SearchWA

	sets.Path = tmpDir
	sets.HttpAuth = true
	sets.SearchWA = searchWA

	t.Cleanup(func() {
		sets.Path = prevPath
		sets.HttpAuth = prevHTTPAuth
		sets.SearchWA = prevSearchWA
	})

	r := gin.New()
	auth.SetupAuth(r)
	SetupRoute(r)

	return r
}

func basicHeader(user, pass string) string {
	token := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))

	return "Basic " + token
}
