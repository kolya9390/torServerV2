package api

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

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
	t.Cleanup(func() {
		SetShutdownHook(nil)
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

func TestShutdownHandlerUsesGracefulHook(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var calls int32

	ConfigureShutdown("local", "")
	SetShutdownHook(func() {
		atomic.AddInt32(&calls, 1)
	})

	t.Cleanup(func() {
		ConfigureShutdown("local", "")
		SetShutdownHook(nil)
	})

	route := gin.New()
	route.Use(servicesMiddleware(newAPIServicesFixture(t, nil)))
	route.POST("/shutdown", shutdown)

	req := httptest.NewRequest(http.MethodPost, "/shutdown", nil)
	req.RemoteAddr = "127.0.0.1:12345"

	w := httptest.NewRecorder()
	route.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		if atomic.LoadInt32(&calls) == 1 {
			return
		}

		select {
		case <-deadline:
			t.Fatal("expected graceful shutdown hook to be called")
		case <-ticker.C:
		}
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
