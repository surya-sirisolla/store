package handlers

import (
	"net/http"
	"strings"
	"time"

	"store/backend/internal/secrets"
	"store/backend/internal/services"

	"github.com/gin-gonic/gin"
)

// LivekeepingHandler exposes the Livekeeping stock-item integration: reading and
// saving the credentials, and running the sync that pulls the catalog into our DB.
type LivekeepingHandler struct {
	svc      *services.LivekeepingService
	credFile string
}

func NewLivekeepingHandler(svc *services.LivekeepingService, credFile string) *LivekeepingHandler {
	return &LivekeepingHandler{svc: svc, credFile: credFile}
}

// GetConfig returns the saved integration settings without exposing the token
// (only a masked preview + whether one is configured), plus the cached result of
// the last token-validity check so the console can flag an expired token.
func (h *LivekeepingHandler) GetConfig(c *gin.Context) {
	cfg := secrets.ReadLivekeeping(h.credFile)
	c.JSON(http.StatusOK, gin.H{
		"configured":       cfg.Configured(),
		"token_preview":    secrets.Masked(cfg.Token),
		"company_id":       cfg.CompanyID,
		"user_id":          cfg.UserID,
		"last_sync_at":     cfg.LastSyncAt,
		"last_sync_count":  cfg.LastSyncCount,
		"token_valid":      cfg.TokenValid,
		"token_checked_at": cfg.TokenCheckedAt,
		"token_error":      cfg.TokenError,
		// Sync scope — resolved booleans (nil in storage ⇒ enabled).
		"sync_stock":   cfg.StockEnabled(),
		"sync_godowns": cfg.GodownsEnabled(),
		"sync_profile": cfg.ProfileEnabled(),
	})
}

// SaveConfig persists the credentials. A blank token keeps the existing one, so
// the owner can edit the company/user ids without re-pasting the token.
func (h *LivekeepingHandler) SaveConfig(c *gin.Context) {
	var in struct {
		Token     string `json:"token"`
		CompanyID string `json:"company_id"`
		UserID    string `json:"user_id"`
		// Sync-scope toggles. Pointers so an omitted field leaves the current
		// value untouched (partial updates), while an explicit false disables it.
		SyncStock   *bool `json:"sync_stock"`
		SyncGodowns *bool `json:"sync_godowns"`
		SyncProfile *bool `json:"sync_profile"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	cfg := secrets.ReadLivekeeping(h.credFile)
	if t := strings.TrimSpace(in.Token); t != "" {
		cfg.Token = t
		// A freshly pasted token has an unknown validity until re-checked.
		cfg.TokenValid = nil
		cfg.TokenCheckedAt = ""
		cfg.TokenError = ""
	}
	if v := strings.TrimSpace(in.CompanyID); v != "" {
		cfg.CompanyID = v
	}
	if v := strings.TrimSpace(in.UserID); v != "" {
		cfg.UserID = v
	}
	if in.SyncStock != nil {
		cfg.SyncStock = in.SyncStock
	}
	if in.SyncGodowns != nil {
		cfg.SyncGodowns = in.SyncGodowns
	}
	if in.SyncProfile != nil {
		cfg.SyncProfile = in.SyncProfile
	}

	if err := secrets.WriteLivekeeping(h.credFile, cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "saved"})
}

// Check verifies the saved token against Livekeeping and caches the result so
// the console can show valid/expired without hitting the API on every load.
func (h *LivekeepingHandler) Check(c *gin.Context) {
	cfg := secrets.ReadLivekeeping(h.credFile)
	if !cfg.Configured() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paste your Livekeeping token first"})
		return
	}

	err := h.svc.CheckToken(c.Request.Context(), services.SyncCreds{
		Token: cfg.Token, CompanyID: cfg.CompanyID, UserID: cfg.UserID,
	})

	valid := err == nil
	cfg.TokenValid = &valid
	cfg.TokenCheckedAt = time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		cfg.TokenError = err.Error()
	} else {
		cfg.TokenError = ""
	}
	_ = secrets.WriteLivekeeping(h.credFile, cfg)

	c.JSON(http.StatusOK, gin.H{
		"token_valid":      valid,
		"token_checked_at": cfg.TokenCheckedAt,
		"token_error":      cfg.TokenError,
	})
}

