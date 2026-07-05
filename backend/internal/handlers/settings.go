package handlers

import (
	"net/http"
	"strings"

	"store/backend/internal/secrets"

	"github.com/gin-gonic/gin"
)

// SettingsHandler manages owner-editable runtime settings (the LLM providers).
type SettingsHandler struct {
	keysFile string
}

func NewSettingsHandler(keysFile string) *SettingsHandler {
	return &SettingsHandler{keysFile: keysFile}
}

// keyIn is the request shape for one provider selection.
type keyIn struct {
	Provider secrets.Provider `json:"provider"`
	Key      string           `json:"key"`
	BaseURL  string           `json:"base_url"`
	Model    string           `json:"model"`
}

// keyView is the safe (masked) representation returned to the console. The raw
// key is never returned; base URL + model are safe to show for local providers.
type keyView struct {
	Provider secrets.Provider `json:"provider"`
	Masked   string           `json:"masked"`
	BaseURL  string           `json:"base_url,omitempty"`
	Model    string           `json:"model,omitempty"`
}

func viewOf(k secrets.LLMKey) keyView {
	v := keyView{Provider: k.Provider, BaseURL: k.BaseURL, Model: k.Model}
	if k.Provider == secrets.ProviderLocal {
		v.Masked = k.Model // for local, the "identity" shown is the model name
	} else {
		v.Masked = secrets.Masked(k.Key)
	}
	return v
}

// GetLLMKeys reports the configured providers and masked previews. Never
// returns the raw keys.
func (h *SettingsHandler) GetLLMKeys(c *gin.Context) {
	cfg := secrets.ReadLLMConfig(h.keysFile)

	resp := gin.H{
		"configured": cfg.Primary.Configured(),
		"source":     llmSource(h.keysFile, cfg.Primary.Configured()),
		"primary":    nil,
		"fallback":   nil,
	}
	if cfg.Primary.Configured() {
		resp["primary"] = viewOf(cfg.Primary)
	}
	if cfg.Fallback != nil && cfg.Fallback.Configured() {
		resp["fallback"] = viewOf(*cfg.Fallback)
	}
	c.JSON(http.StatusOK, resp)
}

// validateKey checks one provider selection and returns the cleaned LLMKey, or
// an error message describing what's wrong.
func validateKey(label string, in keyIn) (secrets.LLMKey, string) {
	if !secrets.ValidProvider(in.Provider) {
		return secrets.LLMKey{}, label + ": choose a valid provider"
	}
	if in.Provider == secrets.ProviderLocal {
		base := strings.TrimSpace(in.BaseURL)
		model := strings.TrimSpace(in.Model)
		if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
			return secrets.LLMKey{}, label + ": local needs a base URL like http://host.docker.internal:11434/v1"
		}
		if model == "" {
			return secrets.LLMKey{}, label + ": pick a local model"
		}
		return secrets.LLMKey{Provider: secrets.ProviderLocal, BaseURL: base, Model: model}, ""
	}
	if len(strings.TrimSpace(in.Key)) < 10 {
		return secrets.LLMKey{}, label + ": that doesn't look like a valid API key"
	}
	return secrets.LLMKey{Provider: in.Provider, Key: strings.TrimSpace(in.Key)}, ""
}

// SetLLMKeys saves/updates the primary and optional fallback providers
// (owner-only). The bot picks up the change and (re)starts within a few seconds.
func (h *SettingsHandler) SetLLMKeys(c *gin.Context) {
	var in struct {
		Primary  keyIn  `json:"primary"`
		Fallback *keyIn `json:"fallback"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	primary, errMsg := validateKey("primary", in.Primary)
	if errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errMsg})
		return
	}
	cfg := secrets.LLMConfig{Primary: primary}

	// A fallback is present only if the caller sent one with usable contents.
	if in.Fallback != nil && (strings.TrimSpace(in.Fallback.Key) != "" || strings.TrimSpace(in.Fallback.BaseURL) != "") {
		fallback, fErr := validateKey("fallback", *in.Fallback)
		if fErr != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": fErr})
			return
		}
		if fallback.Provider == primary.Provider {
			c.JSON(http.StatusBadRequest, gin.H{"error": "fallback must use a different provider than the primary"})
			return
		}
		cfg.Fallback = &fallback
	}

	if err := secrets.WriteLLMConfig(h.keysFile, cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "saved"})
}

// DeleteLLMKeys removes the saved providers (owner-only).
func (h *SettingsHandler) DeleteLLMKeys(c *gin.Context) {
	if err := secrets.DeleteLLMConfig(h.keysFile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// PromoteFallback makes the secondary provider the primary and demotes the old
// primary to secondary (a swap), so the owner can shift which key/source the bot
// uses first without re-entering anything.
func (h *SettingsHandler) PromoteFallback(c *gin.Context) {
	cfg := secrets.ReadLLMConfig(h.keysFile)
	if cfg.Fallback == nil || !cfg.Fallback.Configured() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no secondary provider to promote"})
		return
	}
	demoted := cfg.Primary
	next := secrets.LLMConfig{Primary: *cfg.Fallback}
	if demoted.Configured() {
		next.Fallback = &demoted
	}
	if err := secrets.WriteLLMConfig(h.keysFile, next); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "promoted"})
}

// DeleteFallback removes only the secondary provider, leaving the primary intact.
func (h *SettingsHandler) DeleteFallback(c *gin.Context) {
	cfg := secrets.ReadLLMConfig(h.keysFile)
	if cfg.Fallback == nil {
		c.JSON(http.StatusOK, gin.H{"message": "deleted"})
		return
	}
	cfg.Fallback = nil
	if err := secrets.WriteLLMConfig(h.keysFile, cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// DeletePrimary removes the primary provider. If a secondary exists it's promoted
// to primary so the bot keeps working; otherwise all providers are cleared.
func (h *SettingsHandler) DeletePrimary(c *gin.Context) {
	cfg := secrets.ReadLLMConfig(h.keysFile)
	if cfg.Fallback != nil && cfg.Fallback.Configured() {
		next := secrets.LLMConfig{Primary: *cfg.Fallback}
		if err := secrets.WriteLLMConfig(h.keysFile, next); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "primary removed; secondary promoted"})
		return
	}
	// No secondary to fall back on — clear everything (env fallback, if any, applies).
	if err := secrets.DeleteLLMConfig(h.keysFile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func llmSource(path string, configured bool) string {
	switch {
	case !configured:
		return "none"
	case secrets.FromFile(path):
		return "console"
	default:
		return "environment"
	}
}
