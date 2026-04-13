package api

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthorizeShutdownRequestLocalMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ConfigureShutdown("local", "")
	SetShutdownHook(nil)

	t.Cleanup(func() {
		ConfigureShutdown("local", "")
		SetShutdownHook(nil)
	})

	t.Run("allows loopback", func(t *testing.T) {
		c := testShutdownContext("127.0.0.1:12345", "")

		if err := authorizeShutdownRequest(c); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("denies remote ip", func(t *testing.T) {
		c := testShutdownContext("10.10.10.10:12345", "")

		err := authorizeShutdownRequest(c)
		if err == nil {
			t.Fatal("expected forbidden error")
		}

		apiErr, ok := err.(APIError)
		if !ok || apiErr.Status != http.StatusForbidden {
			t.Fatalf("expected forbidden APIError, got %T %v", err, err)
		}
	})
}

func TestAuthorizeShutdownRequestPublicMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ConfigureShutdown("public", "secret-token")
	SetShutdownHook(nil)

	t.Cleanup(func() {
		ConfigureShutdown("local", "")
		SetShutdownHook(nil)
	})

	t.Run("denies missing token", func(t *testing.T) {
		c := testShutdownContext("203.0.113.8:4567", "")

		err := authorizeShutdownRequest(c)
		if err == nil {
			t.Fatal("expected unauthorized error")
		}

		apiErr, ok := err.(APIError)
		if !ok || apiErr.Status != http.StatusUnauthorized {
			t.Fatalf("expected unauthorized APIError, got %T %v", err, err)
		}
	})

	t.Run("allows token header", func(t *testing.T) {
		c := testShutdownContext("203.0.113.8:4567", "secret-token")

		if err := authorizeShutdownRequest(c); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestRequestShutdownUsesHookOnce(t *testing.T) {
	var calls int32

	SetShutdownHook(func() {
		atomic.AddInt32(&calls, 1)
	})

	if !RequestShutdown() {
		t.Fatal("expected shutdown request to be accepted")
	}

	if !RequestShutdown() {
		t.Fatal("expected repeated shutdown request to be accepted")
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected hook call count=1, got %d", got)
	}
}

func testShutdownContext(remoteAddr, token string) *gin.Context {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/shutdown", nil)
	req.RemoteAddr = remoteAddr

	if token != "" {
		req.Header.Set("X-TS-Shutdown-Token", token)
	}

	c.Request = req

	return c
}
