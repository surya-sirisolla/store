package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	// DatabaseURL, when set, is a full Postgres connection URL (e.g. a Neon
	// string like postgresql://user:pass@host/db?sslmode=require) and takes
	// precedence over the discrete DB_* values below. Lets the same binary point
	// at a managed cloud Postgres without changing code.
	DatabaseURL string

	DBHost       string
	DBPort       string
	DBUser       string
	DBPassword   string
	DBName       string
	Port         string
	ClaudeAPIKey string

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

	// WhatsAppResetFile, when present, tells the PicoClaw supervisor to delete
	// the whatsmeow pairing store and restart with a fresh QR — a full "remove
	// connection", as opposed to BotDisabledFile which only pauses.
	WhatsAppResetFile string

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

	// AdminUser / AdminPassword are the console's single login, supplied at
	// setup time the way Grafana takes GF_SECURITY_ADMIN_USER/PASSWORD. They
	// seed the credential on the FIRST boot only (hashed with bcrypt); after
	// that the database is the source of truth and AdminPassword is ignored —
	// rotate it from the console's Security page. AdminPassword has no default
	// and the server refuses to start without it, so no deployment ever runs on
	// a well-known password.
	AdminUser     string
	AdminPassword string

	// JWTSecret signs the session tokens issued at login. Optional: when unset,
	// the server reads (or generates and persists) a secret at JWTSecretFile so
	// a stock deployment needs only ADMIN_PASSWORD configured.
	JWTSecret     string
	JWTSecretFile string
}

func Load() *Config {
	return &Config{
		DatabaseURL:       getEnv("DATABASE_URL", ""),
		DBHost:            getEnv("DB_HOST", "localhost"),
		DBPort:            getEnv("DB_PORT", "5432"),
		DBUser:            getEnv("DB_USER", "storeuser"),
		DBPassword:        getEnv("DB_PASSWORD", "storepass"),
		DBName:            getEnv("DB_NAME", "storedb"),
		Port:              getEnv("PORT", "8080"),
		ClaudeAPIKey:      getEnv("CLAUDE_API_KEY", getEnv("ANTHROPIC_API_KEY", "")),
		PicoclawLogPath:   getEnv("PICOCLAW_LOG_PATH", "/shared/picoclaw.log"),
		AnthropicKeyFile:  getEnv("ANTHROPIC_KEY_FILE", "/shared/anthropic_key"),
		LLMKeysFile:       getEnv("LLM_KEYS_FILE", "/shared/llm_keys.json"),
		BotDisabledFile:   getEnv("BOT_DISABLED_FILE", "/shared/bot_disabled"),
		WhatsAppResetFile: getEnv("WHATSAPP_RESET_FILE", "/shared/whatsapp_reset"),
		LivekeepingFile:   getEnv("LIVEKEEPING_FILE", "/shared/livekeeping.json"),
		SessionsDir:       getEnv("SESSIONS_DIR", "/picoclaw-data/workspace/sessions"),
		SessionIdleHours:  getEnvInt("SESSION_IDLE_HOURS", 24),
		InternalToken:     getEnv("INTERNAL_TOKEN", "store-internal"),
		PicoclawURL:       getEnv("PICOCLAW_URL", "http://picoclaw:18790"),
		AdminUser:         getEnv("ADMIN_USER", "admin"),
		AdminPassword:     getEnv("ADMIN_PASSWORD", ""),
		JWTSecret:         getEnv("JWT_SECRET", ""),
		JWTSecretFile:     getEnv("JWT_SECRET_FILE", "/shared/jwt_secret"),
	}
}

// DSN returns the Postgres connection string. A DATABASE_URL (e.g. Neon) wins;
// otherwise it's assembled from the discrete DB_* values for a local install.
func (c *Config) DSN() string {
	if c.DatabaseURL != "" {
		return c.DatabaseURL
	}
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
