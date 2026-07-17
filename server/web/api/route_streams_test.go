package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	sets "server/settings"

	"github.com/gin-gonic/gin"
)

func TestExplicitStreamsRoutesRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if err := SetupRouteWithServices(
		r,
		func() sets.RuntimeState { return sets.RuntimeState{} },
		newAPIServicesFixture(t, nil),
		"test",
	); err != nil {
		t.Fatalf("SetupRouteWithServices returned error: %v", err)
	}

	cases := []string{
		"/streams/stat",
		"/streams/m3u",
		"/streams/play",
	}

	for _, path := range cases {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound {
			t.Fatalf("route %s is not registered", path)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/streams/save", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatalf("route %s is not registered", "/streams/save")
	}
}
