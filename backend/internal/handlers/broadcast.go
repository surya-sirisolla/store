package handlers

import (
	"net/http"
	"strings"

	"store/backend/internal/services"

	"github.com/gin-gonic/gin"
)

// BroadcastHandler will host the reminder/broadcast feature. For now it exposes
// a single owner-only "send a test message" action used to verify the outbound
// WhatsApp path end-to-end before wiring up scheduled broadcasts.
type BroadcastHandler struct {
	sender *services.WhatsAppSender
}

func NewBroadcastHandler(sender *services.WhatsAppSender) *BroadcastHandler {
	return &BroadcastHandler{sender: sender}
}

// TestSend delivers one message to a single number, so the owner can confirm the
// bot can send outbound before scheduling a broadcast to a list.
func (h *BroadcastHandler) TestSend(c *gin.Context) {
	var in struct {
		To   string `json:"to"`
		Text string `json:"text"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if strings.TrimSpace(in.To) == "" || strings.TrimSpace(in.Text) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a phone number and message are required"})
		return
	}
	if err := h.sender.Send(c.Request.Context(), in.To, in.Text); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "sent"})
}
