package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"server/log"
	"server/settings"
)

// SetupAuth enables passive auth parsing middleware when HTTP auth is configured.
func SetupAuth(engine *gin.Engine) {
	if !settings.HTTPAuth {
		return
	}

	engine.Use(BasicAuth(loadAccounts()))
}

func loadAccounts() gin.Accounts {
	path := filepath.Join(settings.Path, "accs.db")
	buf, err := os.ReadFile(path)

	if err != nil {
		log.TLogln("auth accounts file not found:", path)

		return gin.Accounts{}
	}

	accounts := gin.Accounts{}
	if err := json.Unmarshal(buf, &accounts); err != nil {
		log.TLogln("Error parse accs.db", err)

		return gin.Accounts{}
	}

	return accounts
}

// BasicAuth marks request as requiring auth and attaches authenticated user if credentials are valid.
func BasicAuth(accounts gin.Accounts) gin.HandlerFunc {
	credentials := make(map[string]string, len(accounts))
	for user, password := range accounts {
		credentials[authorizationHeader(user, password)] = user
	}

	return func(c *gin.Context) {
		c.Set("auth_required", true)

		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if user, ok := credentials[header]; ok {
			c.Set(gin.AuthUserKey, user)
		}
	}
}

// CheckAuth enforces authentication for protected routes.
func CheckAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !settings.HTTPAuth {
			return
		}

		if _, ok := c.Get(gin.AuthUserKey); ok {
			return
		}

		c.Header("WWW-Authenticate", "Basic realm=Authorization Required")
		c.AbortWithStatus(http.StatusUnauthorized)
	}
}

func authorizationHeader(user, password string) string {
	base := user + ":" + password

	return "Basic " + base64.StdEncoding.EncodeToString([]byte(base))
}
