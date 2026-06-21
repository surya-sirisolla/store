// Package console is a synchronous HTTP-ingress channel that lets the owner
// console chat with the agent over a simple request/reply HTTP call, reusing
// the same agent brain, model fallback and MCP tools as the WhatsApp bot.
//
// Unlike the async chat channels, this one bridges picoclaw's inbound/outbound
// message bus back into a single HTTP response: the webhook handler publishes an
// inbound message, registers a per-chat reply waiter, and blocks until Send()
// delivers the agent's final answer for that chat (or a timeout fires).
//
// The owner is injected as a trusted caller ("console-owner") so the staff-only
// directory MCP tools unlock. The MCP server must treat that same sentinel as
// the owner (see store/backend/cmd/mcp/main.go). The webhook is guarded by a
// shared secret (auth_token, or the INTERNAL_TOKEN env var) which the trusted
// store-backend supplies when proxying — the browser never sees it.
package console

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/identity"
	"github.com/sipeed/picoclaw/pkg/logger"
)

const (
	channelType = "console"
	webhookPath = "/webhook/console"

	// ownerCaller is forwarded as the trusted MCP "_caller" so the console
	// unlocks the staff-only directory tools. MUST stay in sync with the
	// owner sentinel in store/backend/cmd/mcp/main.go.
	ownerCaller = "console-owner"

	defaultTimeoutSeconds = 90
)

// ConsoleSettings is the channel's "settings" block in config.json.
type ConsoleSettings struct {
	// AuthToken is the shared secret callers must present in the
	// X-Internal-Token header. If empty, the INTERNAL_TOKEN env var is used.
	AuthToken string `json:"auth_token"`
	// TimeoutSeconds caps how long a request waits for the agent's reply.
	TimeoutSeconds int `json:"timeout_seconds"`
}

// ConsoleChannel implements channels.Channel and channels.WebhookHandler.
type ConsoleChannel struct {
	*channels.BaseChannel
	authToken string
	timeout   time.Duration

	mu      sync.Mutex
	waiters map[string]chan string // chatID -> final-reply channel
}

func init() {
	// Make "console" a valid, decodable channel type without editing the
	// in-tree config registry.
	config.RegisterChannelSettings(channelType, ConsoleSettings{})

	channels.RegisterFactory(channelType, func(channelName, _ string, cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		bc := cfg.Channels[channelName]
		if bc == nil {
			return nil, fmt.Errorf("channel %q: config not found", channelName)
		}
		decoded, err := bc.GetDecoded()
		if err != nil {
			return nil, err
		}
		settings, _ := decoded.(*ConsoleSettings)
		ch := NewConsoleChannel(settings, b)
		if channelName != channelType {
			ch.SetName(channelName)
		}
		return ch, nil
	})
}

// NewConsoleChannel builds a console channel from its settings.
func NewConsoleChannel(s *ConsoleSettings, b *bus.MessageBus) *ConsoleChannel {
	token := ""
	timeoutSeconds := defaultTimeoutSeconds
	if s != nil {
		token = strings.TrimSpace(s.AuthToken)
		if s.TimeoutSeconds > 0 {
			timeoutSeconds = s.TimeoutSeconds
		}
	}
	if token == "" {
		token = strings.TrimSpace(os.Getenv("INTERNAL_TOKEN"))
	}

	base := channels.NewBaseChannel(
		channelType,
		s,
		b,
		[]string{"*"}, // gated by the auth token, not an allow-list
		channels.WithMaxMessageLength(0),
	)

	return &ConsoleChannel{
		BaseChannel: base,
		authToken:   token,
		timeout:     time.Duration(timeoutSeconds) * time.Second,
		waiters:     make(map[string]chan string),
	}
}

func (c *ConsoleChannel) Start(_ context.Context) error {
	if c.authToken == "" {
		logger.WarnC("console", "console channel has no auth_token / INTERNAL_TOKEN — all requests will be rejected")
	}
	logger.InfoCF("console", "Starting console channel (owner web chat)", map[string]any{"path": webhookPath})
	c.SetRunning(true)
	return nil
}

func (c *ConsoleChannel) Stop(_ context.Context) error {
	logger.InfoC("console", "Stopping console channel")
	c.SetRunning(false)
	return nil
}

// WebhookPath mounts the ingress on the shared gateway HTTP server.
func (c *ConsoleChannel) WebhookPath() string { return webhookPath }

// ServeHTTP accepts {message, session_id}, runs one agent turn, and returns
// {reply} synchronously.
func (c *ConsoleChannel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c.authToken == "" || !tokenEqual(r.Header.Get("X-Internal-Token"), c.authToken) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Message   string `json:"message"`
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		sessionID = "default"
	}
	chatID := "console-" + sessionID

	reply := make(chan string, 1)
	c.mu.Lock()
	c.waiters[chatID] = reply
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.waiters, chatID)
		c.mu.Unlock()
	}()

	inboundCtx := bus.InboundContext{
		Channel:  channelType,
		ChatID:   chatID,
		ChatType: "direct",
		SenderID: ownerCaller,
		Raw:      map[string]string{"caller_phone": ownerCaller},
	}
	sender := bus.SenderInfo{
		Platform:    channelType,
		PlatformID:  ownerCaller,
		CanonicalID: identity.BuildCanonicalID(channelType, ownerCaller),
		DisplayName: "Owner",
	}
	c.HandleInboundContext(r.Context(), chatID, message, nil, inboundCtx, sender)

	timer := time.NewTimer(c.timeout)
	defer timer.Stop()
	select {
	case text := <-reply:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"reply": text})
	case <-timer.C:
		http.Error(w, "assistant timed out", http.StatusGatewayTimeout)
	case <-r.Context().Done():
		http.Error(w, "client closed request", 499)
	}
}

// Send receives outbound messages from the agent and delivers the final answer
// to the waiting HTTP request. Auxiliary chatter (tool calls/feedback/thoughts)
// carries a message_kind and is skipped.
func (c *ConsoleChannel) Send(_ context.Context, msg bus.OutboundMessage) ([]string, error) {
	if !c.IsRunning() {
		return nil, channels.ErrNotRunning
	}
	if msg.Context.Raw != nil && strings.TrimSpace(msg.Context.Raw["message_kind"]) != "" {
		return nil, nil
	}
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil, nil
	}

	c.mu.Lock()
	reply := c.waiters[msg.ChatID]
	c.mu.Unlock()
	if reply != nil {
		select {
		case reply <- content:
		default: // already answered for this turn
		}
	}
	return nil, nil
}

func tokenEqual(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
