package auth

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"server/log"
)

const (
	// AuthUserKey is the key used to store authenticated username in gin context.
	AuthUserKey = "auth_user"
	// AuthRequiredKey is the key used to mark routes requiring authentication.
	AuthRequiredKey = "auth_required"
)

// BasicAuthMiddleware creates a gin middleware that validates HTTP Basic Auth
// against the provided Store.
func BasicAuthMiddleware(store *Store, enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled {
			c.Next()

			return
		}

		c.Set(AuthRequiredKey, true) // marks route as requiring auth

		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if header == "" {
			c.Next()

			return
		}

		username, password, ok := parseBasicAuth(header)
		if !ok {
			c.Next()

			return
		}

		if err := store.VerifyPassword(username, password); err != nil {
			if err != ErrUserNotFound {
				log.TLogln("Auth verification error:", err)
			}

			c.Next()

			return
		}

		c.Set(AuthUserKey, username)
		c.Next()
	}
}

// RequireAuth is a gin handler that enforces authentication on protected routes.
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if required, ok := c.Get(AuthRequiredKey); ok {
			if authRequired, _ := required.(bool); authRequired { //nolint:errcheck
				if _, exists := c.Get(AuthUserKey); !exists {
					c.Header("WWW-Authenticate", "Basic realm=\"TorrServer\"")
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
						"error": "authentication required",
					})

					return
				}
			}
		}

		c.Next()
	}
}

// parseBasicAuth extracts username and password from an HTTP Basic Authorization header.
func parseBasicAuth(header string) (string, string, bool) {
	if !strings.HasPrefix(header, "Basic ") {
		return "", "", false
	}

	decoded, err := base64.StdEncoding.DecodeString(header[6:])
	if err != nil {
		return "", "", false
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}

	return parts[0], parts[1], true
}
