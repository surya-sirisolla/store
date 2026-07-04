package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DBHost       string
	DBPort       string
	DBUser       string
	DBPassword   string
	DBName       string
	Port         string
	ClaudeAPIKey string

	// The single business owner record, seeded on first boot. No login — see
	// models.User.
	OwnerName  string
	OwnerEmail string

	// PicoclawLogPath is the shared log file the WhatsApp bot mirrors its output
	// to, read by the console to show pairing status + QR.
	PicoclawLogPath string

	// AnthropicKeyFile is the legacy shared file for a single Anthropic key.
	AnthropicKeyFile string

	// LLMKeysFile is the shared JSON file holding the owner-set LLM provider
	// keys (primary + optional fallback). Read by both the API and the bot.
	LLMKeysFile string

	// BotDisabledFile, when present, pauses the WhatsApp bot (owner toggle).
	BotDisabledFile string

	// LivekeepingFile is the shared JSON file holding the Livekeeping stock-sync
	// credentials (token + company/user ids) and last-sync bookkeeping.
	LivekeepingFile string

	// SessionsDir is the bot's per-chat conversation store (picoclaw's
	// <workspace>/sessions), shared into this container so the idle-chat cleanup
	// job can prune it. SessionIdleHours is how long a chat may be untouched
	// before that job clears it.
	SessionsDir      string
	SessionIdleHours int

	// InternalToken guards internal-only endpoints (e.g. the bot's contact
	// ingestion) that are reachable on the docker network but not user-facing.
	InternalToken string

	// PicoclawURL is the base URL of the PicoClaw gateway on the docker network.
	// The owner-console assistant proxies chat to its console channel.
	PicoclawURL string

	// OwnerPassword gates the owner-console login (POST /api/auth/login). Set
	// this before exposing the console publicly.
	OwnerPassword string

	// JWTSecret signs the session tokens issued at login.
	JWTSecret string
}

func Load() *Config {
	return &Config{
		DBHost:           getEnv("DB_HOST", "localhost"),
		DBPort:           getEnv("DB_PORT", "5432"),
		DBUser:           getEnv("DB_USER", "storeuser"),
		DBPassword:       getEnv("DB_PASSWORD", "storepass"),
		DBName:           getEnv("DB_NAME", "storedb"),
		Port:             getEnv("PORT", "8080"),
		ClaudeAPIKey:     getEnv("CLAUDE_API_KEY", getEnv("ANTHROPIC_API_KEY", "")),
		OwnerName:        getEnv("OWNER_NAME", "Owner"),
		OwnerEmail:       getEnv("OWNER_EMAIL", "owner@business.local"),
		PicoclawLogPath:  getEnv("PICOCLAW_LOG_PATH", "/shared/picoclaw.log"),
		AnthropicKeyFile: getEnv("ANTHROPIC_KEY_FILE", "/shared/anthropic_key"),
		LLMKeysFile:      getEnv("LLM_KEYS_FILE", "/shared/llm_keys.json"),
		BotDisabledFile:  getEnv("BOT_DISABLED_FILE", "/shared/bot_disabled"),
		LivekeepingFile:  getEnv("LIVEKEEPING_FILE", "/shared/livekeeping.json"),
		SessionsDir:      getEnv("SESSIONS_DIR", "/picoclaw-data/workspace/sessions"),
		SessionIdleHours: getEnvInt("SESSION_IDLE_HOURS", 24),
		InternalToken:    getEnv("INTERNAL_TOKEN", "store-internal"),
		PicoclawURL:      getEnv("PICOCLAW_URL", "http://picoclaw:18790"),
		OwnerPassword:    getEnv("OWNER_PASSWORD", ""),
		JWTSecret:        getEnv("JWT_SECRET", ""),
	}
}

func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName,
	)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
