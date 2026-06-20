package config

import (
	"fmt"
	"os"
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

	// InternalToken guards internal-only endpoints (e.g. the bot's contact
	// ingestion) that are reachable on the docker network but not user-facing.
	InternalToken string
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
		InternalToken:    getEnv("INTERNAL_TOKEN", "store-internal"),
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
