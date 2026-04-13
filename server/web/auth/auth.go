package auth

import (
	"net/http"
	"strings"

	"encoding/base64"

	"github.com/gin-gonic/gin"

	"server/auth"

	"server/log"
	"server/settings"

	bbolt "go.etcd.io/bbolt"
)

var (
	authStore   *auth.Store
	tokenStore  *auth.TokenStore
	authEnabled bool
)

// InitFromStore initializes auth from a pre-configured store (for testing or custom setups).
func InitFromStore(s *auth.Store, enabled bool) {
	authStore = s
	authEnabled = enabled
	tokenStore = auth.NewTokenStore(nil)
}

// InitAuth initializes the auth package with the BBolt database.
// Performs migration from legacy accs.db if needed.
func InitAuth() {
	tdb := settings.NewTDB()
	if tdb == nil {
		log.TLogln("Auth: failed to get BBolt DB, auth disabled")

		return
	}

	rawDB := tdb.GetRawDB()

	bboltDB, ok := rawDB.(*bbolt.DB)
	if !ok || bboltDB == nil {
		log.TLogln("Auth: raw DB is nil or wrong type, auth disabled")

		return
	}

	authStore = auth.NewStore(bboltDB)
	tokenStore = auth.NewTokenStore(bboltDB)
	authEnabled = settings.HTTPAuth

	// Run migration from legacy accs.db
	if err := auth.MigrateFromAccsDB(authStore, settings.Path); err != nil {
		log.TLogln("Auth migration error:", err)
	}

	// Ensure shutdown token exists
	if err := tokenStore.EnsureDefaultToken(); err != nil {
		log.TLogln("Auth: shutdown token init error:", err)
	}
}

// GetAuthStore returns the auth store for API handlers.
func GetAuthStore() *auth.Store {
	return authStore
}

// GetTokenStore returns the token store for API handlers.
func GetTokenStore() *auth.TokenStore {
	return tokenStore
}

// IsAuthEnabled returns true if HTTP auth is enabled.
func IsAuthEnabled() bool {
	return authEnabled
}

// SetupAuth enables passive auth parsing middleware.
func SetupAuth(engine *gin.Engine) {
	if authStore == nil {
		return
	}

	engine.Use(auth.BasicAuthMiddleware(authStore, authEnabled))
}

// CheckAuth enforces authentication for protected routes.
func CheckAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authEnabled {
			c.Next()

			return
		}

		if _, ok := c.Get(auth.AuthUserKey); !ok {
			c.Header("WWW-Authenticate", "Basic realm=\"TorrServer\"")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "authentication required",
			})

			return
		}

		c.Next()
	}
}

// GetShutdownToken returns the stored shutdown token.
func GetShutdownToken() (string, error) {
	if tokenStore == nil {
		return "", nil
	}

	return tokenStore.GetShutdownToken()
}

// BasicAuthMiddlewareForTest creates a simple auth middleware for testing (accepts single user/pass).
func BasicAuthMiddlewareForTest(user, pass string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if strings.HasPrefix(header, "Basic ") {
			decoded, err := base64.StdEncoding.DecodeString(header[6:])
			if err != nil {
				c.Next()

				return
			}

			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 && parts[0] == user && parts[1] == pass {
				c.Set(auth.AuthUserKey, user)
			}
		}

		c.Next()
	}
}
