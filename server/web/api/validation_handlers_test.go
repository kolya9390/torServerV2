package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"server/internal/app/contracts"
)

func TestSettingsValidationRequiresAction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(servicesMiddleware(newAPIServicesFixture(t, nil)))
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
	r.Use(servicesMiddleware(newAPIServicesFixture(t, nil)))
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
	r.Use(servicesMiddleware(newAPIServicesFixture(t, nil)))
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
	r.Use(servicesMiddleware(newAPIServicesFixture(t, nil)))
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

func TestViewedListUsesInjectedService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	viewedSvc := &viewedListStub{
		items: []*contracts.ViewedItem{{Hash: "abc", FileIndex: 2}},
	}
	r.Use(servicesMiddleware(newAPIServicesFixture(t, &contracts.APIServices{Viewed: viewedSvc})))
	r.POST("/viewed", viewed)

	req := httptest.NewRequest(http.MethodPost, "/viewed", strings.NewReader(`{"action":"list","hash":"abc"}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if viewedSvc.requestedHash != "abc" {
		t.Fatalf("expected injected viewed service to receive hash abc, got %q", viewedSvc.requestedHash)
	}

	if !strings.Contains(w.Body.String(), `"file_index":2`) {
		t.Fatalf("expected viewed response from injected service, got %s", w.Body.String())
	}
}

type viewedListStub struct {
	requestedHash string
	items         []*contracts.ViewedItem
}

func (s *viewedListStub) SetViewed(*contracts.ViewedItem) {}

func (s *viewedListStub) RemoveViewed(*contracts.ViewedItem) {}

func (s *viewedListStub) ListViewed(hash string) []*contracts.ViewedItem {
	s.requestedHash = hash

	return s.items
}

func TestStorageValidationRejectsInvalidValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(servicesMiddleware(newAPIServicesFixture(t, nil)))
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
	r.Use(servicesMiddleware(newAPIServicesFixture(t, nil)))
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
