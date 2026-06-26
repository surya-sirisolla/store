package handlers

import (
	"net/http"
	"strings"

	"store/backend/internal/auth"

	"github.com/gin-gonic/gin"
)

// AuthHandler issues session tokens for the single-operator console login.
type AuthHandler struct {
	password string
	secret   string
}

func NewAuthHandler(password, secret string) *AuthHandler {
	return &AuthHandler{password: password, secret: secret}
}

// Login checks the shared admin password and, if correct, returns a signed
// session token the console stores and sends back as a Bearer token.
func (h *AuthHandler) Login(c *gin.Context) {
	var in struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if !auth.CheckPassword(in.Password, h.password) {
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
