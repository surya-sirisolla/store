// Package secrets manages the runtime-editable LLM provider keys, stored as a
// JSON file on the shared volume so both the API server and the PicoClaw bot
// can read it. The owner configures a primary provider key and an optional
// fallback provider key (e.g. Claude primary, Gemini fallback); the bot uses
// the fallback automatically when the primary fails.
//
// For backward compatibility, when no JSON config exists the ANTHROPIC_API_KEY
// environment variable is treated as a Claude-only primary key.
package secrets

import (
	"encoding/json"
	"os"
	"strings"
)

// Provider identifies an LLM vendor. These string values are the stable
// contract shared with the frontend dropdown and the PicoClaw init script.
type Provider string

const (
	ProviderClaude Provider = "claude"
	ProviderGemini Provider = "gemini"
	ProviderOpenAI Provider = "openai"
	ProviderLocal  Provider = "local" // self-hosted, OpenAI-compatible (Ollama, LM Studio…)
)

// ValidProvider reports whether p is one of the supported providers.
func ValidProvider(p Provider) bool {
	switch p {
	case ProviderClaude, ProviderGemini, ProviderOpenAI, ProviderLocal:
		return true
	}
	return false
}

// LLMKey is a single provider selection. Cloud providers use Key; a local
// provider uses BaseURL + Model instead (no API key needed).
type LLMKey struct {
	Provider Provider `json:"provider"`
	Key      string   `json:"key,omitempty"`
	BaseURL  string   `json:"base_url,omitempty"` // local/custom endpoint, e.g. http://host.docker.internal:11434/v1
	Model    string   `json:"model,omitempty"`    // local/custom model id, e.g. llama3.1
}

// Configured reports whether this key is usable: a cloud provider needs a key;
// a local provider needs a base URL.
func (k LLMKey) Configured() bool {
	if k.Provider == ProviderLocal {
		return strings.TrimSpace(k.BaseURL) != ""
	}
	return strings.TrimSpace(k.Key) != ""
}

// LLMConfig is the owner's full key setup: a primary and an optional fallback.
type LLMConfig struct {
	Primary  LLMKey  `json:"primary"`
	Fallback *LLMKey `json:"fallback,omitempty"`
}

// ReadLLMConfig loads the config from the JSON file. If the file is missing or
// empty, it falls back to the ANTHROPIC_API_KEY env var as a Claude primary
// (so existing .env-based setups keep working). Returns a zero LLMConfig if
// nothing is configured.
func ReadLLMConfig(path string) LLMConfig {
	if b, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(b)) != "" {
		var cfg LLMConfig
		if json.Unmarshal(b, &cfg) == nil && cfg.Primary.Configured() {
			return cfg
		}
	}
	if env := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); env != "" {
		return LLMConfig{Primary: LLMKey{Provider: ProviderClaude, Key: env}}
	}
	return LLMConfig{}
}

// WriteLLMConfig saves the config to the JSON file (owner-set).
func WriteLLMConfig(path string, cfg LLMConfig) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// DeleteLLMConfig removes the config file so the env fallback (if any) applies.
func DeleteLLMConfig(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// HasPrimaryKey reports whether any usable primary provider is configured.
func HasPrimaryKey(path string) bool {
	return ReadLLMConfig(path).Primary.Configured()
}

// FromFile reports whether the config is set via the file (vs only env / unset).
func FromFile(path string) bool {
	b, err := os.ReadFile(path)
	return err == nil && strings.TrimSpace(string(b)) != ""
}

// ReadAnthropic returns the Claude/Anthropic key if one is configured as either
// the primary or fallback provider, else the ANTHROPIC_API_KEY env var. Used by
// the backend's AI extraction feature, which only speaks Anthropic's API.
func ReadAnthropic(path string) string {
	cfg := ReadLLMConfig(path)
	if cfg.Primary.Provider == ProviderClaude && cfg.Primary.Key != "" {
		return cfg.Primary.Key
	}
	if cfg.Fallback != nil && cfg.Fallback.Provider == ProviderClaude && cfg.Fallback.Key != "" {
		return cfg.Fallback.Key
	}
	return strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
}

// MigrateLegacyKey lifts a pre-existing single Anthropic key file into the new
// JSON config (as a Claude primary) when no JSON config exists yet. No-op
// otherwise. Safe to call on every startup so owners who set a key under the
// old single-key flow keep it without re-entering.
func MigrateLegacyKey(legacyPath, keysFile string) {
	if FromFile(keysFile) {
		return
	}
	b, err := os.ReadFile(legacyPath)
	if err != nil {
		return
	}
	key := strings.TrimSpace(string(b))
	if key == "" {
		return
	}
	_ = WriteLLMConfig(keysFile, LLMConfig{Primary: LLMKey{Provider: ProviderClaude, Key: key}})
}

// Masked renders a key for display without exposing it, e.g. "sk-ant…aB12".
func Masked(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if len(key) <= 10 {
		return "••••"
	}
	return key[:6] + "…" + key[len(key)-4:]
}
