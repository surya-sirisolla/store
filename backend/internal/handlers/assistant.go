package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// AssistantHandler powers the owner-console chat. It does not run an agent
// itself — it proxies the owner's message to the PicoClaw console channel
// (same agent brain, model fallback and MCP tools as the WhatsApp bot),
// attaching the shared internal token so the browser never holds it.
type AssistantHandler struct {
	picoclawURL string
	token       string
	client      *http.Client
}

func NewAssistantHandler(picoclawURL, token string) *AssistantHandler {
	return &AssistantHandler{
		picoclawURL: strings.TrimRight(picoclawURL, "/"),
		token:       token,
		// Slightly longer than the console channel's own reply timeout so the
		// agent's timeout surfaces as a clean message rather than a proxy cut-off.
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// Status reports whether the PicoClaw gateway (and thus the assistant) is
// reachable, so the console can show an offline state instead of failing chats.
func (h *AssistantHandler) Status(c *gin.Context) {
	req, _ := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, h.picoclawURL+"/health", nil)
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"online": false})
		return
	}
	defer resp.Body.Close()
	c.JSON(http.StatusOK, gin.H{"online": resp.StatusCode < 500})
}

// Chat forwards one message to the console channel and returns the agent's reply.
func (h *AssistantHandler) Chat(c *gin.Context) {
	var in struct {
		Message   string `json:"message"`
		SessionID string `json:"session_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if strings.TrimSpace(in.Message) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message is required"})
		return
	}

	body, _ := json.Marshal(map[string]string{"message": in.Message, "session_id": in.SessionID})
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, h.picoclawURL+"/webhook/console", bytes.NewReader(body))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not build request"})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", h.token)

	resp, err := h.client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "The assistant is offline. Make sure the WhatsApp bot is enabled (it shares the same engine)."})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(readLimited(resp.Body, 512)))
		if resp.StatusCode == http.StatusGatewayTimeout {
			msg = "The assistant took too long to respond. Try a simpler question."
		} else if msg == "" {
			msg = "The assistant returned an error."
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": msg})
		return
	}

	var out struct {
		Reply string `json:"reply"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "invalid response from assistant"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"reply": out.Reply})
}

func readLimited(r io.Reader, n int64) []byte {
	b, _ := io.ReadAll(io.LimitReader(r, n))
	return b
}
