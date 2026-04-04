package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestErrorResponderWrapsAbortWithError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ErrorResponder())
	r.GET("/boom", func(c *gin.Context) {
		_ = c.AbortWithError(http.StatusBadRequest, errors.New("bad request payload"))
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"type":"error"`) {
		t.Fatalf("expected error envelope, got %s", w.Body.String())
	}
}

func TestErrorResponderWithTypedError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ErrorResponder())
	r.GET("/boom", func(c *gin.Context) {
		_ = c.AbortWithError(http.StatusBadRequest, newValidationError("hash", "is required"))
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"type":"validation_error"`) || !strings.Contains(body, `"field":"hash"`) {
		t.Fatalf("expected typed envelope, got %s", body)
	}
}
