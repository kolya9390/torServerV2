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

const runtimeContextKey = "auth_runtime"

type Runtime struct {
	store  *auth.Store
	token  *auth.TokenStore
	authOn bool
}

func (r *Runtime) Store() *auth.Store {
	if r == nil {
		return nil
	}

	return r.store
}

func (r *Runtime) TokenStore() *auth.TokenStore {
	if r == nil {
		return nil
	}

	return r.token
}

func (r *Runtime) Enabled() bool {
	return r != nil && r.authOn
}

func NewRuntimeFromStore(store *auth.Store, tokens *auth.TokenStore, enabled bool) *Runtime {
	if tokens == nil {
		tokens = auth.NewTokenStore(nil)
	}

	return &Runtime{
		store:  store,
		token:  tokens,
		authOn: enabled,
	}
}

// InitFromStore initializes auth from a pre-configured store (for testing or custom setups).
func InitFromStore(s *auth.Store, enabled bool) {
	runtime := NewRuntimeFromStore(s, nil, enabled)
	authStore = runtime.Store()
	authEnabled = runtime.Enabled()
	tokenStore = runtime.TokenStore()
}

func NewRuntimeWithRuntimeState(runtimeState func() settings.RuntimeState) *Runtime {
	if runtimeState == nil {
		runtimeState = func() settings.RuntimeState { return settings.RuntimeState{} }
	}

	runtimePath := runtimeState().PathConfig().Path

	tdb := settings.NewTDBAtPath(runtimePath)
	if tdb == nil {
		log.TLogln("Auth: failed to get BBolt DB, auth disabled")

		return &Runtime{}
	}

	rawDB := tdb.GetRawDB()

	bboltDB, ok := rawDB.(*bbolt.DB)
	if !ok || bboltDB == nil {
		log.TLogln("Auth: raw DB is nil or wrong type, auth disabled")

		return &Runtime{}
	}

	store := auth.NewStore(bboltDB)
	tokens := auth.NewTokenStore(bboltDB)
	authCfg := runtimeState().AuthConfig()

	if err := auth.MigrateFromAccsDB(store, runtimePath); err != nil {
		log.TLogln("Auth migration error:", err)
	}

	if err := tokens.EnsureDefaultToken(); err != nil {
		log.TLogln("Auth: shutdown token init error:", err)
	}

	return &Runtime{
		store:  store,
		token:  tokens,
		authOn: authCfg.HTTPAuth,
	}
}

// InitAuthWithRuntimeState initializes the auth package with the BBolt database.
// Performs migration from legacy accs.db if needed.
func InitAuthWithRuntimeState(runtimeState func() settings.RuntimeState) {
	runtime := NewRuntimeWithRuntimeState(runtimeState)
	authStore = runtime.Store()
	tokenStore = runtime.TokenStore()
	authEnabled = runtime.Enabled()
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

func SetupAuthRuntime(engine *gin.Engine, runtime *Runtime) {
	engine.Use(RuntimeMiddleware(runtime))

	if runtime == nil || runtime.Store() == nil {
		return
	}

	engine.Use(auth.BasicAuthMiddleware(runtime.Store(), runtime.Enabled()))
}

func RuntimeMiddleware(runtime *Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		if runtime != nil {
			c.Set(runtimeContextKey, runtime)
		}

		c.Next()
	}
}

func RuntimeFromContext(c *gin.Context) *Runtime {
	if c != nil {
		if value, ok := c.Get(runtimeContextKey); ok {
			if runtime, ok := value.(*Runtime); ok {
				return runtime
			}
		}
	}

	return &Runtime{
		store:  authStore,
		token:  tokenStore,
		authOn: authEnabled,
	}
}

// CheckAuth enforces authentication for protected routes.
func CheckAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		runtime := RuntimeFromContext(c)
		if !runtime.Enabled() {
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
