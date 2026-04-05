package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSettingsValidationRequiresAction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/settings", settings)

	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), `"field":"action"`) {
		t.Fatalf("expected action validation error, got %s", w.Body.String())
	}
}

func TestTorrentsValidationRequiresAction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/torrents", torrents)

	req := httptest.NewRequest(http.MethodPost, "/torrents", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), `"field":"action"`) {
		t.Fatalf("expected action validation error, got %s", w.Body.String())
	}
}

func TestCacheValidationRequiresHash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/cache", cache)

	req := httptest.NewRequest(http.MethodPost, "/cache", strings.NewReader(`{"action":"get"}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), `"field":"hash"`) {
		t.Fatalf("expected hash validation error, got %s", w.Body.String())
	}
}

func TestViewedValidationRequiresPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/viewed", viewed)

	req := httptest.NewRequest(http.MethodPost, "/viewed", strings.NewReader(`{"action":"set"}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), `"field":"viewed"`) {
		t.Fatalf("expected viewed validation error, got %s", w.Body.String())
	}
}

func TestStorageValidationRejectsInvalidValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/storage/settings", UpdateStorageSettings)

	req := httptest.NewRequest(http.MethodPost, "/storage/settings", strings.NewReader(`{"settings":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), `"field":"settings"`) {
		t.Fatalf("expected settings validation error, got %s", w.Body.String())
	}
}

func TestCacheNotFoundEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/cache", cache)

	req := httptest.NewRequest(http.MethodPost, "/cache", strings.NewReader(`{"action":"get","hash":"0123456789abcdef0123456789abcdef01234567"}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), `"type":"not_found"`) {
		t.Fatalf("expected not_found envelope, got %s", w.Body.String())
	}
}
