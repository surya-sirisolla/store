package secrets

import (
	"encoding/json"
	"os"
	"strings"
)

// Default Livekeeping account identifiers for this deployment. They're baked in
// as sensible defaults (the owner can override them from the console) so the
// only thing the owner strictly has to paste in is the session token.
const (
	defaultLivekeepingCompanyID = "6a01644b3ac5cdc66cd604ea"
	defaultLivekeepingUserID    = "658e9bb67fb495449a9a3097"
)

// LivekeepingConfig holds the credentials + last-sync bookkeeping for the
// Livekeeping stock-items integration. Stored as JSON on the shared volume,
// same as the LLM keys, so it survives restarts.
type LivekeepingConfig struct {
	Token         string `json:"token"`
	CompanyID     string `json:"company_id"`
	UserID        string `json:"user_id"`
	LastSyncAt    string `json:"last_sync_at,omitempty"`
	LastSyncCount int    `json:"last_sync_count,omitempty"`

	// Cached result of the last token validity check (the token is a Livekeeping
	// session token that expires). Kept here — "saved temporarily" — so the
	// console can show valid/expired at a glance without re-hitting the API on
	// every page load. Refreshed by an explicit check and by every sync run
	// (a sync that gets 401/403 flips this to false). nil = never checked.
	TokenValid     *bool  `json:"token_valid,omitempty"`
	TokenCheckedAt string `json:"token_checked_at,omitempty"`
	TokenError     string `json:"token_error,omitempty"`

	// Sync scope — which parts of Livekeeping each sync run (manual or scheduled)
	// imports. Pointers so an older config with these absent is treated as "all
	// enabled" (the previous behavior); nil ⇒ enabled.
	SyncStock   *bool `json:"sync_stock,omitempty"`
	SyncGodowns *bool `json:"sync_godowns,omitempty"`
	SyncProfile *bool `json:"sync_profile,omitempty"`
}

// Configured reports whether a token is present (the one thing sync needs).
func (c LivekeepingConfig) Configured() bool {
	return strings.TrimSpace(c.Token) != ""
}

// StockEnabled / GodownsEnabled / ProfileEnabled report whether that part is in
// scope for a sync. A nil pointer (older config) means enabled.
func (c LivekeepingConfig) StockEnabled() bool   { return c.SyncStock == nil || *c.SyncStock }
func (c LivekeepingConfig) GodownsEnabled() bool { return c.SyncGodowns == nil || *c.SyncGodowns }
func (c LivekeepingConfig) ProfileEnabled() bool { return c.SyncProfile == nil || *c.SyncProfile }

// ReadLivekeeping loads the config, filling in the default company/user ids when
// they're blank so callers always get a usable payload.
func ReadLivekeeping(path string) LivekeepingConfig {
	var cfg LivekeepingConfig
	if b, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(b)) != "" {
		_ = json.Unmarshal(b, &cfg)
	}
	if strings.TrimSpace(cfg.CompanyID) == "" {
		cfg.CompanyID = defaultLivekeepingCompanyID
	}
	if strings.TrimSpace(cfg.UserID) == "" {
		cfg.UserID = defaultLivekeepingUserID
	}
	return cfg
}

// WriteLivekeeping saves the config to the JSON file (owner-set).
func WriteLivekeeping(path string, cfg LivekeepingConfig) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
