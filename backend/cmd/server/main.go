// Command server runs the Owner-console HTTP API. One deployment serves one
// business, with a single admin login configured at setup time.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"strings"
	"time"

	"store/backend/internal/auth"
	"store/backend/internal/config"
	"store/backend/internal/db"
	"store/backend/internal/handlers"
	"store/backend/internal/secrets"
	"store/backend/internal/services"
	"store/backend/internal/store"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	cfg := config.Load()
	// Validate the setup-time config before touching the database, so a missing
	// password fails on a fresh install without having migrated anything.
	requireAdminPassword(cfg)
	resolveJWTSecret(cfg)
	database := db.Connect(cfg.DSN())

	// Carry over a key set under the old single-Anthropic-key flow.
	secrets.MigrateLegacyKey(cfg.AnthropicKeyFile, cfg.LLMKeysFile)

	st := store.New(database)
	// Seed the console's single admin login from ADMIN_USER/ADMIN_PASSWORD.
	seedAdmin(st, cfg)
	// Resolve the Claude key at call time for the AI extraction feature: the
	// owner-set keys file wins (primary or fallback claude key), else env.
	keyFunc := func() string { return secrets.ReadAnthropic(cfg.LLMKeysFile) }

	claude := services.NewClaudeService(keyFunc)
	excel := services.NewExcelService(database, claude)
	aiImport := services.NewAIImportService(st, claude)
	livekeeping := services.NewLivekeepingService(database, st)
	sender := services.NewWhatsAppSender(cfg.PicoclawURL, cfg.InternalToken)
	broadcast := services.NewBroadcastService(st, sender)
	jobs := services.NewJobService(database, st, livekeeping, broadcast, cfg.LivekeepingFile, cfg.SessionsDir, cfg.SessionIdleHours)
	jobs.Start()

	consoleH := handlers.NewConsoleHandler(database, st)
	livekeepingH := handlers.NewLivekeepingHandler(livekeeping, cfg.LivekeepingFile)
	jobsH := handlers.NewJobsHandler(jobs)
	alertsH := handlers.NewAlertsHandler(database, st)
	broadcastH := handlers.NewBroadcastHandler(sender)
	bulkH := handlers.NewBulkHandler(database, excel, aiImport)
	aiImportH := handlers.NewAIImportHandler(aiImport)
	waH := handlers.NewWhatsAppHandler(cfg.PicoclawLogPath, cfg.LLMKeysFile, cfg.BotDisabledFile, cfg.WhatsAppResetFile)
	settingsH := handlers.NewSettingsHandler(cfg.LLMKeysFile)
	ingestH := handlers.NewIngestHandler(st, cfg.InternalToken)
	assistantH := handlers.NewAssistantHandler(cfg.PicoclawURL, cfg.InternalToken)
	authH := handlers.NewAuthHandler(st, cfg.JWTSecret)

	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowAllOrigins: true,
		AllowMethods:    []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:    []string{"Origin", "Content-Type", "Authorization"},
	}))

	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	// Internal: the bot reports every inbound message here (token-guarded).
	r.POST("/internal/bot/contact-seen", ingestH.ContactSeen)

	// Login is the one public /api route; everything else requires the session
	// token it issues. Rate-limited per IP to blunt brute-force attempts.
	r.POST("/api/auth/login", handlers.RateLimit(10, 5*time.Minute), authH.Login)

	// Console API — every route requires a valid session token.
	api := r.Group("/api")
	api.Use(handlers.RequireAuth(cfg.JWTSecret))
	{
		// Who am I — the console reads this on load.
		api.GET("/me", authH.Me)

		api.GET("/stats", consoleH.Stats)

		api.GET("/categories", consoleH.ListCategories)
		api.POST("/categories", consoleH.CreateCategory)
		api.DELETE("/categories/:id", consoleH.DeleteCategory)

		api.GET("/listings", consoleH.ListListings)
		api.POST("/listings", consoleH.CreateListing)
		api.PUT("/listings/:id", consoleH.UpdateListing)
		api.DELETE("/listings/:id", consoleH.DeleteListing)

		api.POST("/bulk/inspect", bulkH.InspectUpload)
		api.POST("/bulk/import", bulkH.ConfirmImport)
		api.GET("/bulk/jobs", bulkH.ListJobs)
		api.GET("/bulk/jobs/:id", bulkH.JobStatus)
		api.POST("/bulk/ai-import/estimate", bulkH.AIImportEstimate)
		api.POST("/bulk/ai-import", bulkH.AIImportExcel)

		api.POST("/listings/ai-import", aiImportH.Import)

		// Livekeeping stock-item sync: credentials + token validity. The actual
		// run is driven through the scheduled-jobs endpoints below.
		api.GET("/integrations/livekeeping", livekeepingH.GetConfig)
		api.PUT("/integrations/livekeeping", livekeepingH.SaveConfig)
		api.POST("/integrations/livekeeping/check", livekeepingH.Check)

		// Automation: scheduled background jobs (system jobs + owner reminders).
		api.GET("/jobs", jobsH.List)
		api.POST("/jobs", jobsH.Create)
		api.POST("/jobs/preview", jobsH.Preview)
		api.PUT("/jobs/:key", jobsH.Update)
		api.DELETE("/jobs/:key", jobsH.Delete)
		api.PUT("/jobs/:key/schedule", jobsH.SaveSchedule)
		api.POST("/jobs/:key/run", jobsH.Run)

		// Outbound WhatsApp: send a one-off test message (foundation for broadcasts).
		api.POST("/bot/whatsapp/test-send", broadcastH.TestSend)

		// Rotate the console-login password (requires the current one).
		api.POST("/auth/change-password", authH.ChangePassword)

		api.GET("/business-profile", consoleH.GetProfile)
		api.PUT("/business-profile", consoleH.UpdateProfile)

		// Business locations (godowns synced from Livekeeping) with owner-editable
		// addresses.
		api.GET("/business-locations", consoleH.ListLocations)
		api.PUT("/business-locations/:id", consoleH.UpdateLocation)

		api.GET("/staff", consoleH.ListStaff)
		api.POST("/staff", consoleH.CreateStaff)
		api.DELETE("/staff/:id", consoleH.DeleteStaff)

		api.GET("/bot/stats", consoleH.BotStats)
		api.GET("/bot/activity", consoleH.BotActivityFeed)
		api.GET("/bot/contacts", consoleH.BotContacts)
		api.GET("/bot/contact-activity", consoleH.BotContactActivity)
		api.GET("/bot/whatsapp/status", waH.Status)
		api.POST("/bot/whatsapp/enable", waH.Enable)
		api.POST("/bot/whatsapp/disable", waH.Disable)
		api.POST("/bot/whatsapp/remove", waH.Remove)

		// Customer alerts (waitlist + restock-ready). List/counts/status/re-check.
		api.GET("/alerts", alertsH.List)
		api.GET("/alerts/counts", alertsH.Counts)
		api.PATCH("/alerts/:id/status", alertsH.SetStatus)
		api.POST("/alerts/recheck", alertsH.Recheck)

		api.POST("/assistant/chat", assistantH.Chat)
		api.GET("/assistant/status", assistantH.Status)

		api.GET("/settings/llm-keys", settingsH.GetLLMKeys)
		api.PUT("/settings/llm-keys", settingsH.SetLLMKeys)
		api.DELETE("/settings/llm-keys", settingsH.DeleteLLMKeys)
		api.POST("/settings/llm-keys/promote", settingsH.PromoteFallback)
		api.DELETE("/settings/llm-keys/primary", settingsH.DeletePrimary)
		api.DELETE("/settings/llm-keys/fallback", settingsH.DeleteFallback)
		api.GET("/settings/local-llm", settingsH.DetectLocalLLM)
	}

	log.Printf("owner-console API listening on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}

// requireAdminPassword refuses to start without a console password. There is no
// default, so a deployment can never come up on a well-known credential the way
// it could when the super-admin password fell back to a hardcoded value.
func requireAdminPassword(cfg *config.Config) {
	if strings.TrimSpace(cfg.AdminPassword) == "" {
		log.Fatal("ADMIN_PASSWORD is not set. Choose the console's admin password in .env before starting, " +
			"e.g.: ADMIN_PASSWORD=some-long-passphrase (and optionally ADMIN_USER, which defaults to \"admin\").")
	}
}

// resolveJWTSecret settles the session-signing secret. An explicit JWT_SECRET
// wins; otherwise the secret is read from (or generated once and persisted to)
// JWTSecretFile on the shared volume, so a stock deployment needs only
// ADMIN_PASSWORD configured and sessions still survive a restart. Only a
// secret that can neither be read nor written is fatal — an ephemeral one would
// silently invalidate every session on each restart.
func resolveJWTSecret(cfg *config.Config) {
	if strings.TrimSpace(cfg.JWTSecret) != "" {
		return
	}
	if b, err := os.ReadFile(cfg.JWTSecretFile); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		cfg.JWTSecret = strings.TrimSpace(string(b))
		return
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		log.Fatalf("generating a session secret: %v", err)
	}
	secret := hex.EncodeToString(buf)
	if err := os.WriteFile(cfg.JWTSecretFile, []byte(secret), 0o600); err != nil {
		log.Fatalf("JWT_SECRET is not set and a generated one could not be saved to %s: %v\n"+
			"Set JWT_SECRET in .env instead, e.g.: openssl rand -hex 32", cfg.JWTSecretFile, err)
	}
	cfg.JWTSecret = secret
	log.Printf("generated a session secret and saved it to %s", cfg.JWTSecretFile)
}

// seedAdmin creates the console's single admin login on first boot from
// ADMIN_USER/ADMIN_PASSWORD, the way Grafana takes its admin credentials. It is
// idempotent: once the credential exists the env password is ignored (rotate it
// from the console's Security page), though a changed ADMIN_USER is applied.
//
// ADMIN_PASSWORD has no default and is required, so a deployment can never come
// up on a well-known password.
func seedAdmin(st *store.Store, cfg *config.Config) {
	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		log.Fatalf("hashing the admin password: %v", err)
	}
	created, err := st.EnsureAdmin(context.Background(), cfg.AdminUser, hash)
	if err != nil {
		log.Fatalf("seeding the admin account: %v", err)
	}
	if created {
		log.Printf("seeded console admin %q", cfg.AdminUser)
	}
}
