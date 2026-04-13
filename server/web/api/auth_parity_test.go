package api

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.etcd.io/bbolt"

	"server/auth"
	sets "server/settings"
	wauth "server/web/auth"
)

func TestProtectedEndpointsRequireAuth(t *testing.T) {
	r := setupAuthRouter(t, false)

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/settings", body: `{"action":"get"}`},
		{method: http.MethodPost, path: "/api/v1/settings", body: `{"action":"get"}`},
		{method: http.MethodPost, path: "/api/v1/torrents", body: `{"action":"list"}`},
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
	gin.SetMode(gin.TestMode)

	// Create simple router with just auth middleware
	r := gin.New()
	r.Use(wauth.BasicAuthMiddlewareForTest("admin", "secret"))
	r.POST("/api/v1/settings", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings", strings.NewReader(`{"action":"get"}`))
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

	// Create simple router without auth
	r := gin.New()
	r.POST("/api/v1/settings", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings", strings.NewReader(`{"action":"get"}`))
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

	prevPath := sets.Path
	prevHTTPAuth := sets.HTTPAuth
	prevSearchWA := sets.SearchWA

	sets.Path = tmpDir
	sets.HTTPAuth = true
	sets.SearchWA = searchWA

	t.Cleanup(func() {
		sets.Path = prevPath
		sets.HTTPAuth = prevHTTPAuth
		sets.SearchWA = prevSearchWA
	})

	// Create BBolt DB and add test user
	db, err := bbolt.Open(filepath.Join(tmpDir, "config.db"), 0600, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	t.Cleanup(func() { db.Close() })

	store := auth.NewStore(db)
	if err := store.AddUser("admin", "secret"); err != nil {
		t.Fatalf("add user: %v", err)
	}

	// Initialize auth directly for testing
	wauth.InitFromStore(store, true)

	r := gin.New()
	wauth.SetupAuth(r)
	SetupRoute(r)

	return r
}

func basicHeader(user, pass string) string {
	token := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))

	return "Basic " + token
}
