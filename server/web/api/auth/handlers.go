package authapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"server/web/auth"
)

// RegisterAuthRoutes registers auth management API endpoints.
func RegisterAuthRoutes(rg *gin.RouterGroup) {
	users := rg.Group("/auth/users")
	{
		users.GET("", listUsers)
		users.POST("", addUser)
		users.DELETE("/:name", removeUser)
	}

	config := rg.Group("/config")
	{
		config.GET("/shutdown-token", getShutdownToken)
		config.POST("/shutdown-token", setShutdownToken)
		config.POST("/shutdown-token/generate", generateShutdownToken)
	}
}

// @Summary List users
// @Description Returns list of usernames and creation dates (no passwords).
// @Tags Auth
// @Produce json
// @Success 200 {object} map[string]string
// @Router /api/v1/auth/users [get].
func listUsers(c *gin.Context) {
	store := auth.GetAuthStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auth store not initialized"})

		return
	}

	users, err := store.ListUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})

		return
	}

	result := make(map[string]string)
	for name, createdAt := range users {
		result[name] = createdAt.Format("2006-01-02T15:04:05Z")
	}

	c.JSON(http.StatusOK, result)
}

type addUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"` //nolint:gosec // user creation requires a password
}

// @Summary Add user
// @Description Creates a new user with bcrypt-hashed password.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body addUserRequest true "User credentials"
// @Success 201 {string} string "created"
// @Router /api/v1/auth/users [post].
func addUser(c *gin.Context) {
	store := auth.GetAuthStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auth store not initialized"})

		return
	}

	var req addUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})

		return
	}

	if err := store.AddUser(req.Username, req.Password); err != nil {
		status := http.StatusBadRequest
		c.JSON(status, gin.H{"error": err.Error()})

		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": "created", "username": req.Username})
}

// @Summary Remove user
// @Description Deletes a user from the auth store.
// @Tags Auth
// @Param name path string true "Username"
// @Success 200 {string} string "removed"
// @Router /api/v1/auth/users/{name} [delete].
func removeUser(c *gin.Context) {
	store := auth.GetAuthStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "auth store not initialized"})

		return
	}

	name := c.Param("name")
	if err := store.RemoveUser(name); err != nil {
		status := http.StatusNotFound
		c.JSON(status, gin.H{"error": err.Error()})

		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "removed", "username": name})
}

// @Summary Get shutdown token info
// @Description Returns whether a shutdown token is configured (never returns the token itself).
// @Tags Config
// @Produce json
// @Success 200 {object} object
// @Router /api/v1/config/shutdown-token [get].
func getShutdownToken(c *gin.Context) {
	store := auth.GetTokenStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "token store not initialized"})

		return
	}

	token, err := store.GetShutdownToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})

		return
	}

	c.JSON(http.StatusOK, gin.H{
		"configured": token != "",
	})
}

type setTokenRequest struct {
	Token string `json:"token" binding:"required,min=16"`
}

// @Summary Set shutdown token
// @Description Sets a new shutdown token. Must be at least 16 characters.
// @Tags Config
// @Accept json
// @Produce json
// @Param request body setTokenRequest true "Token value"
// @Success 200 {string} string "ok"
// @Router /api/v1/config/shutdown-token [post].
func setShutdownToken(c *gin.Context) {
	store := auth.GetTokenStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "token store not initialized"})

		return
	}

	var req setTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})

		return
	}

	if err := store.SetShutdownToken(req.Token); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})

		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// @Summary Generate shutdown token
// @Description Generates a cryptographically secure shutdown token and stores it.
// @Tags Config
// @Produce json
// @Success 200 {object} object
// @Router /api/v1/config/shutdown-token/generate [post].
func generateShutdownToken(c *gin.Context) {
	store := auth.GetTokenStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "token store not initialized"})

		return
	}

	token, err := store.GenerateAndStoreToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})

		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "generated",
		"token":  token,
	})
}
