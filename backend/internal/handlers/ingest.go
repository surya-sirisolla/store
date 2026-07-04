package handlers

import (
	"net/http"
	"strings"

	"store/backend/internal/store"

	"github.com/gin-gonic/gin"
)

// IngestHandler serves internal-only endpoints the bot calls on the docker
// network (not exposed to console users). Guarded by a shared token so that,
// even though the API port is published, only services holding the token can
// post to it.
type IngestHandler struct {
	st    *store.Store
	token string
}

func NewIngestHandler(st *store.Store, token string) *IngestHandler {
	return &IngestHandler{st: st, token: token}
}

// ContactSeen records that a number messaged the bot. PicoClaw calls this for
// every inbound WhatsApp message — independent of whether the AI replies or
// calls any tool — so the owner always sees who tried to reach the business.
func (h *IngestHandler) ContactSeen(c *gin.Context) {
	if c.GetHeader("X-Internal-Token") != h.token {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var in struct {
		Phone   string `json:"phone"`
		Name    string `json:"name"`
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if in.Phone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone required"})
		return
	}
	h.st.RecordContact(c.Request.Context(), in.Phone, in.Name, in.Message)
	// Honor reminder opt-out/opt-in keywords so broadcasts respect "STOP".
	switch strings.ToLower(strings.TrimSpace(in.Message)) {
	case "stop", "unsubscribe", "opt out", "optout":
		h.st.SetContactOptOut(c.Request.Context(), in.Phone, true)
	case "start", "subscribe", "opt in", "optin":
		h.st.SetContactOptOut(c.Request.Context(), in.Phone, false)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
