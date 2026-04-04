package web

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-XSS-Protection", "0")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; media-src 'self' blob:; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")

		if shouldSetHSTS(c) {
			h.Set("Strict-Transport-Security", fmt.Sprintf("max-age=%d; includeSubDomains", getHSTSMaxAge()))
		}
		c.Next()
	}
}

func shouldSetHSTS(c *gin.Context) bool {
	if strings.EqualFold(c.Request.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return c.Request.TLS != nil
}

func getHSTSMaxAge() int {
	const defaultAge = 15552000 // 180 days
	raw := strings.TrimSpace(os.Getenv("TS_HSTS_MAX_AGE"))
	if raw == "" {
		return defaultAge
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return defaultAge
	}
	return v
}
