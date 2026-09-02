package handlers

import (
	"net/http"
	"strings"

	"store/backend/internal/auth"
	"store/backend/internal/store"

	"github.com/gin-gonic/gin"
)

// minPasswordLen is the floor for a new console password.
const minPasswordLen = 8

// ctxUsername is the Gin context key RequireAuth sets from the verified token.
const ctxUsername = "auth_username"

// AuthHandler handles the console's single admin login (username + password,
// seeded from ADMIN_USER/ADMIN_PASSWORD on first boot) and issues session
// tokens. Staff rows in the users table have no password and can't log in.
type AuthHandler struct {
	st     *store.Store
	secret string
}

func NewAuthHandler(st *store.Store, secret string) *AuthHandler {
	return &AuthHandler{st: st, secret: secret}
}

// Login verifies the admin username + password and, if correct, returns a
// signed session token.
func (h *AuthHandler) Login(c *gin.Context) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// A uniform error for "wrong username" and "wrong password" so login can't
	// be used to discover the admin's username.
	const badCreds = "incorrect username or password"

	admin, err := h.st.GetAdmin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify credentials"})
		return
	}
	if admin == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": badCreds})
		return
	}
	// Both checks are evaluated before either is acted on, so a wrong username
	// still pays bcrypt's cost. Short-circuiting here would answer a bad
	// username noticeably faster than a bad password and leak which one was
	// right — defeating the uniform error above.
	userOK := strings.EqualFold(strings.TrimSpace(in.Username), admin.Username)
	passOK := auth.CheckHash(in.Password, admin.PasswordHash)
	if !userOK || !passOK {
		c.JSON(http.StatusUnauthorized, gin.H{"error": badCreds})
		return
	}

	token, err := auth.IssueToken(h.secret, admin.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create session"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "username": admin.Username})
}

// Me returns the authenticated admin's identity — used by the console on load.
func (h *AuthHandler) Me(c *gin.Context) {
	name, _ := c.Get(ctxUsername)
	username, _ := name.(string)
	c.JSON(http.StatusOK, gin.H{"username": username})
}

// ChangePassword rotates the admin's password. It requires the current password
// so a stolen session alone can't silently lock the owner out.
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var in struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if len(strings.TrimSpace(in.NewPassword)) < minPasswordLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "new password must be at least 8 characters"})
		return
	}

	admin, err := h.st.GetAdmin(c.Request.Context())
	if err != nil || admin == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify password"})
		return
	}
	if !auth.CheckHash(in.CurrentPassword, admin.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "current password is incorrect"})
		return
	}

	newHash, err := auth.HashPassword(in.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not set password"})
		return
	}
	if err := h.st.SetAdminPassword(c.Request.Context(), newHash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save password"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "password updated"})
}

// RequireAuth gates the console API behind a valid Bearer session token and
// stashes the admin's username on the gin context.
func RequireAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		token := strings.TrimPrefix(header, "Bearer ")
		if token == "" || token == header {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing session"})
			c.Abort()
			return
		}
		claims, err := auth.ParseToken(token, secret)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired session"})
			c.Abort()
			return
		}
		c.Set(ctxUsername, claims.Username)
		c.Next()
	}
}
