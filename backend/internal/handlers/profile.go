package handlers

import (
	"net/http"

	"store/backend/internal/models"

	"github.com/gin-gonic/gin"
)

// GetProfile returns the single business profile (or an empty object if unset).
func (h *ConsoleHandler) GetProfile(c *gin.Context) {
	p, err := h.st.GetBusinessProfile(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if p == nil {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	c.JSON(http.StatusOK, p)
}

// UpdateProfile upserts the single business profile (owner-only).
func (h *ConsoleHandler) UpdateProfile(c *gin.Context) {
	var input models.BusinessProfile
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if input.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if err := h.st.UpsertBusinessProfile(c.Request.Context(), &input); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, input)
}
