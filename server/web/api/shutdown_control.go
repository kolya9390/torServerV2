package api

import (
	"crypto/subtle"
	"net"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	"server/log"
)

type shutdownMode string

const (
	shutdownModeLocal  shutdownMode = "local"
	shutdownModePublic shutdownMode = "public"
)

type shutdownControl struct {
	mu        sync.RWMutex
	mode      shutdownMode
	token     string
	hook      func()
	requested bool
}

var shutdownCtl = &shutdownControl{
	mode: shutdownModeLocal,
}

// ConfigureShutdown configures shutdown protection mode:
// - local: only localhost requests can trigger shutdown
// - public: requires shutdown token in X-TS-Shutdown-Token (or Bearer token)
func ConfigureShutdown(mode, token string) {
	m := normalizeShutdownMode(mode)
	t := strings.TrimSpace(token)

	shutdownCtl.mu.Lock()
	shutdownCtl.mode = m
	shutdownCtl.token = t
	shutdownCtl.mu.Unlock()

	log.TLogln("Configured shutdown mode:", m)
}

// SetShutdownHook registers graceful process stop callback.
func SetShutdownHook(fn func()) {
	shutdownCtl.mu.Lock()
	shutdownCtl.hook = fn
	shutdownCtl.requested = false
	shutdownCtl.mu.Unlock()
}

// RequestShutdown requests graceful process shutdown using injected runtime hook.
// Returns false when no runtime hook is configured.
func RequestShutdown() bool {
	shutdownCtl.mu.Lock()
	if shutdownCtl.hook == nil {
		shutdownCtl.mu.Unlock()

		return false
	}

	if shutdownCtl.requested {
		shutdownCtl.mu.Unlock()

		return true
	}

	shutdownCtl.requested = true
	fn := shutdownCtl.hook
	shutdownCtl.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			log.TLogln("shutdown hook panic recovered", "panic", r)
		}
	}()

	fn()

	return true
}

func authorizeShutdownRequest(c *gin.Context) error {
	mode, token := currentShutdownConfig()

	switch mode {
	case shutdownModePublic:
		return authorizePublicShutdown(c, token)
	default:
		return authorizeLocalShutdown(c)
	}
}

func currentShutdownConfig() (shutdownMode, string) {
	shutdownCtl.mu.RLock()
	defer shutdownCtl.mu.RUnlock()

	return shutdownCtl.mode, shutdownCtl.token
}

func normalizeShutdownMode(mode string) shutdownMode {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case string(shutdownModePublic):
		return shutdownModePublic
	default:
		return shutdownModeLocal
	}
}

func authorizeLocalShutdown(c *gin.Context) error {
	ip := strings.TrimSpace(c.ClientIP())
	parsed := net.ParseIP(ip)
	if parsed == nil || !parsed.IsLoopback() {
		return newForbiddenError("shutdown in local mode is allowed only from localhost")
	}

	return nil
}

func authorizePublicShutdown(c *gin.Context, token string) error {
	if token == "" {
		return newForbiddenError("shutdown token is not configured")
	}

	receivedToken := strings.TrimSpace(c.GetHeader("X-TS-Shutdown-Token"))
	if receivedToken == "" {
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if len(authorization) > len("Bearer ") && strings.HasPrefix(strings.ToLower(authorization), "bearer ") {
			receivedToken = strings.TrimSpace(authorization[len("Bearer "):])
		}
	}

	if receivedToken == "" {
		return newUnauthorizedError("shutdown token is required")
	}

	if subtle.ConstantTimeCompare([]byte(receivedToken), []byte(token)) != 1 {
		return newUnauthorizedError("invalid shutdown token")
	}

	return nil
}
