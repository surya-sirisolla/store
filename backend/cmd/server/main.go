// Command server runs the Owner-console HTTP API for the single-tenant store.
package main

import (
	"log"

	"store/backend/internal/config"
	"store/backend/internal/db"
	"store/backend/internal/handlers"
	"store/backend/internal/models"
	"store/backend/internal/secrets"
	"store/backend/internal/services"
	"store/backend/internal/store"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

func main() {
	godotenv.Load()

	cfg := config.Load()
	database := db.Connect(cfg.DSN())
	seedOwner(database, cfg)

	// Carry over a key set under the old single-Anthropic-key flow.
	secrets.MigrateLegacyKey(cfg.AnthropicKeyFile, cfg.LLMKeysFile)

	st := store.New(database)
	// Resolve the Claude key at call time for the AI extraction feature: the
	// owner-set keys file wins (primary or fallback claude key), else env.
	keyFunc := func() string { return secrets.ReadAnthropic(cfg.LLMKeysFile) }
	claude := services.NewClaudeService(keyFunc)
	excel := services.NewExcelService(database, claude)
	aiImport := services.NewAIImportService(st, claude)

	consoleH := handlers.NewConsoleHandler(database, st)
	bulkH := handlers.NewBulkHandler(database, excel, aiImport)
	aiImportH := handlers.NewAIImportHandler(aiImport)
	waH := handlers.NewWhatsAppHandler(cfg.PicoclawLogPath, cfg.LLMKeysFile, cfg.BotDisabledFile)
	settingsH := handlers.NewSettingsHandler(cfg.LLMKeysFile)
	ingestH := handlers.NewIngestHandler(st, cfg.InternalToken)
	assistantH := handlers.NewAssistantHandler(cfg.PicoclawURL, cfg.InternalToken)

	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowAllOrigins: true,
		AllowMethods:    []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:    []string{"Origin", "Content-Type", "Authorization"},
	}))

	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	// Internal: the bot reports every inbound message here (token-guarded).
	r.POST("/internal/bot/contact-seen", ingestH.ContactSeen)

	// Single-operator console: no login, the API is open like PicoClaw itself
	// (config/keys gate what it can do, not a session).
	api := r.Group("/api")
	{
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

		api.GET("/business-profile", consoleH.GetProfile)
		api.PUT("/business-profile", consoleH.UpdateProfile)

		api.GET("/staff", consoleH.ListStaff)
		api.POST("/staff", consoleH.CreateStaff)
		api.DELETE("/staff/:id", consoleH.DeleteStaff)

		api.GET("/bot/stats", consoleH.BotStats)
		api.GET("/bot/activity", consoleH.BotActivityFeed)
		api.GET("/bot/contacts", consoleH.BotContacts)
		api.GET("/bot/alerts", consoleH.ListAlerts)
		api.PATCH("/bot/alerts/:id/notified", consoleH.MarkAlertNotified)
		api.GET("/bot/whatsapp/status", waH.Status)
		api.POST("/bot/whatsapp/enable", waH.Enable)
		api.POST("/bot/whatsapp/disable", waH.Disable)

		api.POST("/assistant/chat", assistantH.Chat)
		api.GET("/assistant/status", assistantH.Status)

		api.GET("/settings/llm-keys", settingsH.GetLLMKeys)
		api.PUT("/settings/llm-keys", settingsH.SetLLMKeys)
		api.DELETE("/settings/llm-keys", settingsH.DeleteLLMKeys)
		api.GET("/settings/local-llm", settingsH.DetectLocalLLM)
	}

	log.Printf("owner-console API listening on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}

// seedOwner creates the single owner record on first boot from env config. It
// has no password — there's no login — it just labels who the owner is for
// staff stats and lets the owner's own phone (set later under Staff/Business
// Profile) be recognized by the bot like any other staff number.
func seedOwner(database *gorm.DB, cfg *config.Config) {
	var count int64
	database.Model(&models.User{}).Where("role = ?", models.RoleOwner).Count(&count)
	if count > 0 {
		return
	}
	owner := models.User{
		Name:   cfg.OwnerName,
		Email:  cfg.OwnerEmail,
		Role:   models.RoleOwner,
		Active: true,
	}
	if err := database.Create(&owner).Error; err != nil {
		log.Printf("seed owner: %v", err)
		return
	}
	log.Printf("seeded owner: %s", cfg.OwnerEmail)
}
