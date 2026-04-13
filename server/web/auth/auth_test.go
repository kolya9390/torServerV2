package auth

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"go.etcd.io/bbolt"

	"server/auth"
	"server/settings"
)

func TestAuthMiddlewareIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create temp BBolt DB
	dir := t.TempDir()
	db, err := bbolt.Open(filepath.Join(dir, "test.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := auth.NewStore(db)
	if err := store.AddUser("admin", "secret123"); err != nil {
		t.Fatal(err)
	}

	// Set up auth
	InitAuthFromStore(store, true)

	r := gin.New()
	r.Use(BasicAuthMiddlewareWrapper(store, true))
	r.GET("/protected", CheckAuth(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	// Test unauthenticated
	unauthReq := httptest.NewRequest(http.MethodGet, "/protected", nil)
	unauthW := httptest.NewRecorder()
	r.ServeHTTP(unauthW, unauthReq)

	if unauthW.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated request, got %d", unauthW.Code)
	}

	// Test authenticated
	authReq := httptest.NewRequest(http.MethodGet, "/protected", nil)
	authReq.Header.Set("Authorization", basicAuthHeader("admin", "secret123"))

	authW := httptest.NewRecorder()
	r.ServeHTTP(authW, authReq)

	if authW.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for authenticated request, got %d", authW.Code)
	}
}

func TestAuthDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store, _ := createTempStore(t)
	InitAuthFromStore(store, false)

	r := gin.New()
	r.Use(BasicAuthMiddlewareWrapper(store, false))
	r.GET("/protected", CheckAuth(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 when auth disabled, got %d", w.Code)
	}
}

func createTempStore(t *testing.T) (*auth.Store, *bbolt.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := bbolt.Open(filepath.Join(dir, "test.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	return auth.NewStore(db), db
}

func basicAuthHeader(user, pass string) string {
	token := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))

	return "Basic " + token
}

// InitAuthFromStore initializes auth from a pre-configured store (for testing).
func InitAuthFromStore(s *auth.Store, enabled bool) {
	authStore = s
	authEnabled = enabled
	tokenStore = auth.NewTokenStore(nil) // nil for tests
	_ = settings.Path
}

// BasicAuthMiddlewareWrapper wraps the auth middleware for gin (for testing).
func BasicAuthMiddlewareWrapper(s *auth.Store, enabled bool) gin.HandlerFunc {
	return auth.BasicAuthMiddleware(s, enabled)
}
