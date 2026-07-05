package handlers

import (
	"net/http"
	"strings"

	"store/backend/internal/auth"
	"store/backend/internal/store"

	"github.com/gin-gonic/gin"
)

// minPasswordLen is the floor for a new console password set from the console.
const minPasswordLen = 8

// AuthHandler issues session tokens for the single-operator console login and
// verifies the password against the bcrypt hash stored in the database.
type AuthHandler struct {
	st     *store.Store
	secret string
}

func NewAuthHandler(st *store.Store, secret string) *AuthHandler {
	return &AuthHandler{st: st, secret: secret}
}

// Login checks the supplied password against the stored hash and, if correct,
// returns a signed session token the console stores and sends back as a Bearer
// token.
func (h *AuthHandler) Login(c *gin.Context) {
	var in struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	hash, err := h.st.GetOwnerPasswordHash(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify password"})
		return
	}
	if !auth.CheckHash(in.Password, hash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect password"})
		return
	}
	token, err := auth.IssueToken(h.secret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create session"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}

// ChangePassword rotates the console-login password. It requires the current
// password (so a stolen session alone can't silently lock the owner out) and
// stores the new one as a fresh bcrypt hash. Gated behind a valid session by
// the router.
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

	hash, err := h.st.GetOwnerPasswordHash(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not verify password"})
		return
	}
	if !auth.CheckHash(in.CurrentPassword, hash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "current password is incorrect"})
		return
	}

	newHash, err := auth.HashPassword(in.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not set password"})
		return
	}
	if err := h.st.SetOwnerPassword(c.Request.Context(), newHash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save password"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "password updated"})
}

// RequireAuth gates the owner-console API behind a valid Bearer session token.
func RequireAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		token := strings.TrimPrefix(header, "Bearer ")
		if token == "" || token == header {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing session"})
			c.Abort()
			return
		}
		if err := auth.VerifyToken(token, secret); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired session"})
			c.Abort()
			return
		}
		c.Next()
	}
}
