package auth

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"server/settings"
)

func TestBasicAuthAndCheckAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prevAuth := settings.HttpAuth
	settings.HttpAuth = true
	t.Cleanup(func() {
		settings.HttpAuth = prevAuth
	})

	r := gin.New()
	r.Use(BasicAuth(gin.Accounts{"admin": "secret"}))
	r.GET("/protected", CheckAuth(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	unauthReq := httptest.NewRequest(http.MethodGet, "/protected", nil)
	unauthW := httptest.NewRecorder()
	r.ServeHTTP(unauthW, unauthReq)
	if unauthW.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated request, got %d", unauthW.Code)
	}
	if unauthW.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("expected WWW-Authenticate header for unauthenticated request")
	}

	authReq := httptest.NewRequest(http.MethodGet, "/protected", nil)
	authReq.Header.Set("Authorization", basicAuthHeader("admin", "secret"))
	authW := httptest.NewRecorder()
	r.ServeHTTP(authW, authReq)
	if authW.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for authenticated request, got %d", authW.Code)
	}
}

func basicAuthHeader(user, pass string) string {
	token := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	return "Basic " + token
}
