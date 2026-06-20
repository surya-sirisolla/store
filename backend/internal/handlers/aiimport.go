package handlers

import (
	"net/http"

	"store/backend/internal/services"

	"github.com/gin-gonic/gin"
)

type AIImportHandler struct {
	svc *services.AIImportService
}

func NewAIImportHandler(svc *services.AIImportService) *AIImportHandler {
	return &AIImportHandler{svc: svc}
}

// Import accepts arbitrary raw text and uses Claude to extract listings,
// creating categories/sub-categories and listings (with quantity/price when
// mentioned) directly in Postgres.
func (h *AIImportHandler) Import(c *gin.Context) {
	var input struct {
		RawText string `json:"raw_text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.svc.Import(c.Request.Context(), input.RawText)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}
