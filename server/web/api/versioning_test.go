package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLegacyRouteHasDeprecationHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetupRoute(r)

	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	reqLegacy := httptest.NewRequest(http.MethodGet, "/stream?link=bad&stat=1", nil)
	wLegacy := httptest.NewRecorder()
	r.ServeHTTP(wLegacy, reqLegacy)

	if wLegacy.Header().Get("Deprecation") == "" {
		t.Fatalf("expected Deprecation header on legacy route")
	}

	if wLegacy.Header().Get("Sunset") == "" {
		t.Fatalf("expected Sunset header on legacy route")
	}
}

func TestV1RouteHasNoLegacyDeprecationHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetupRoute(r)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stream?link=bad&stat=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Deprecation"); got != "" {
		t.Fatalf("did not expect legacy Deprecation header on v1 route, got %q", got)
	}
}
