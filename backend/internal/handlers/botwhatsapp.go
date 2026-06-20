package handlers

import (
	"net/http"
	"os"
	"regexp"
	"strings"

	"store/backend/internal/secrets"

	"github.com/gin-gonic/gin"
)

// WhatsAppHandler surfaces PicoClaw's WhatsApp pairing status + QR to the console
// by reading the shared log the bot mirrors its output to.
type WhatsAppHandler struct {
	logPath      string
	keysFile     string
	disabledFile string
}

func NewWhatsAppHandler(logPath, keysFile, disabledFile string) *WhatsAppHandler {
	return &WhatsAppHandler{logPath: logPath, keysFile: keysFile, disabledFile: disabledFile}
}

func (h *WhatsAppHandler) disabled() bool {
	_, err := os.Stat(h.disabledFile)
	return err == nil
}

// Disable pauses the bot (keeps the WhatsApp pairing). Owner-only.
func (h *WhatsAppHandler) Disable(c *gin.Context) {
	if err := os.WriteFile(h.disabledFile, []byte("1"), 0o600); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "bot disabled"})
}

// Enable resumes the bot. Owner-only.
func (h *WhatsAppHandler) Enable(c *gin.Context) {
	if err := os.Remove(h.disabledFile); err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "bot enabled"})
}

// ansi strips terminal color codes so markers parse cleanly.
var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

const (
	markerQR        = "WHATSAPP_QR_CODE="
	markerLogin     = "WHATSAPP_LOGIN_EVENT="
	markerConnected = "WhatsApp native channel connected"
	markerLoggedOut = "WHATSAPP_LOGGED_OUT"
	markerKeyErr    = "api_key is required"
)

// Status reports the WhatsApp connection state and, when pairing is pending, the
// raw QR string for the browser to render.
//
// States: need_key | disabled | starting | waiting_for_scan | connected | logged_out | error
func (h *WhatsAppHandler) Status(c *gin.Context) {
	// Before anything else: no provider key means the bot can't run at all.
	if !secrets.HasPrimaryKey(h.keysFile) {
		c.JSON(http.StatusOK, gin.H{"status": "need_key"})
		return
	}

	// Owner has paused the bot.
	if h.disabled() {
		c.JSON(http.StatusOK, gin.H{"status": "disabled"})
		return
	}

	data, err := os.ReadFile(h.logPath)
	if err != nil {
		// Log not present yet — the bot container is still coming up.
		c.JSON(http.StatusOK, gin.H{"status": "starting", "detail": "bot is starting"})
		return
	}

	qr := ""
	qrIdx, loginSuccessIdx, connIdx, loggedOutIdx, keyErrIdx := -1, -1, -1, -1, -1

	for i, raw := range strings.Split(string(data), "\n") {
		line := ansi.ReplaceAllString(raw, "")
		if idx := strings.Index(line, markerQR); idx >= 0 {
			rest := strings.TrimSpace(line[idx+len(markerQR):])
			if f := strings.Fields(rest); len(f) > 0 {
				qr = f[0]
				qrIdx = i
			}
		}
		if idx := strings.Index(line, markerLogin); idx >= 0 {
			ev := strings.TrimSpace(line[idx+len(markerLogin):])
			if strings.HasPrefix(ev, "success") {
				loginSuccessIdx = i
			}
		}
		if strings.Contains(line, markerConnected) {
			connIdx = i
		}
		if strings.Contains(line, markerLoggedOut) {
			loggedOutIdx = i
		}
		if strings.Contains(line, markerKeyErr) {
			keyErrIdx = i
		}
	}

	// picoclaw logs "connected" as soon as the transport connects, even when
	// it's reusing a stored session that WhatsApp has since revoked (e.g. the
	// owner unlinked the device on their phone) — the rejection arrives a
	// moment later as its own log line. A fresh pairing instead shows up as a
	// login-success event sometime after the QR is printed. So pick whichever
	// marker is most recent (highest line index), not just whichever exists.
	type candidate struct {
		idx    int
		status string
	}
	latest := candidate{idx: -1, status: "starting"}
	for _, cand := range []candidate{
		{qrIdx, "waiting_for_scan"},
		{connIdx, "connected"},
		{loginSuccessIdx, "connected"},
		{loggedOutIdx, "logged_out"},
		{keyErrIdx, "error"},
	} {
		if cand.idx > latest.idx {
			latest = cand
		}
	}

	switch latest.status {
	case "waiting_for_scan":
		c.JSON(http.StatusOK, gin.H{"status": "waiting_for_scan", "qr": qr})
	case "logged_out":
		c.JSON(http.StatusOK, gin.H{
			"status": "logged_out",
			"detail": "WhatsApp was unlinked from this device. Disable then re-enable the bot to get a fresh QR code.",
		})
	case "error":
		c.JSON(http.StatusOK, gin.H{
			"status": "error",
			"detail": "Anthropic API key missing or invalid — set ANTHROPIC_API_KEY in .env and restart.",
		})
	case "connected":
		c.JSON(http.StatusOK, gin.H{"status": "connected"})
	default:
		c.JSON(http.StatusOK, gin.H{"status": "starting", "detail": "bot is starting"})
	}
}
