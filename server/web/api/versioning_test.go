package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sets "server/settings"

	"github.com/gin-gonic/gin"
)

func TestLegacyRouteHasDeprecationHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if err := SetupRouteWithServices(
		r,
		func() sets.RuntimeState { return sets.RuntimeState{} },
		newAPIServicesFixture(t, nil),
		"v1.0.0-test.1",
	); err != nil {
		t.Fatalf("SetupRouteWithServices returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var version versionDocument
	if err := json.Unmarshal(w.Body.Bytes(), &version); err != nil {
		t.Fatalf("decode version document: %v", err)
	}

	if version.Product != apiProduct || version.ApplicationVersion != "v1.0.0-test.1" ||
		version.Current != apiCurrentVersion {
		t.Fatalf("version document = %+v", version)
	}

	if len(version.Capabilities) != 1 || version.Capabilities[0] != apiManagementCapability {
		t.Fatalf("capabilities = %v", version.Capabilities)
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
	if err := SetupRouteWithServices(
		r,
		func() sets.RuntimeState { return sets.RuntimeState{} },
		newAPIServicesFixture(t, nil),
		"v1.0.0-test.1",
	); err != nil {
		t.Fatalf("SetupRouteWithServices returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stream?link=bad&stat=1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Deprecation"); got != "" {
		t.Fatalf("did not expect legacy Deprecation header on v1 route, got %q", got)
	}
}

func TestSetupRouteRejectsMissingApplicationVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	err := SetupRouteWithServices(
		gin.New(),
		func() sets.RuntimeState { return sets.RuntimeState{} },
		newAPIServicesFixture(t, nil),
		" ",
	)
	if err == nil || err.Error() != "api application version is not configured" {
		t.Fatalf("error = %v", err)
	}
}
