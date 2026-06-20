// Package store is the shared data-access layer used by both the HTTP API
// (cmd/server) and the MCP server (cmd/mcp). Every exported method maps to a
// question a WhatsApp user might ask or an action the owner console performs.
//
// The tokenized search is ported from the Signet reference backend
// (internal/store/store.go) and adapted from MongoDB to Postgres/GORM.
package store

import (
	"context"
	"strings"
	"time"

	"store/backend/internal/models"

	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) DB() *gorm.DB { return s.db }

// ── Business profile ──────────────────────────────────────────────────────────

// GetBusinessProfile returns the single business profile row, or nil if unset.
func (s *Store) GetBusinessProfile(ctx context.Context) (*models.BusinessProfile, error) {
	var p models.BusinessProfile
	err := s.db.WithContext(ctx).First(&p).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpsertBusinessProfile writes the single profile row (id is forced to 1).
func (s *Store) UpsertBusinessProfile(ctx context.Context, p *models.BusinessProfile) error {
	p.ID = 1
	return s.db.WithContext(ctx).Save(p).Error
}

// ── Categories ────────────────────────────────────────────────────────────────

// ListCategoryTree returns top-level categories with nested children.
func (s *Store) ListCategoryTree(ctx context.Context) ([]models.Category, error) {
	cats := []models.Category{}
	err := s.db.WithContext(ctx).
		Where("parent_id IS NULL").
		Preload("Children.Children").
		Order("name").
		Find(&cats).Error
	return cats, err
}

// CategoryCount is a category name with how many listings reference it.
type CategoryCount struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

// CategoryCounts lists every category with its listing count (for the bot to
// offer browse options).
func (s *Store) CategoryCounts(ctx context.Context) ([]CategoryCount, error) {
	out := []CategoryCount{}
	err := s.db.WithContext(ctx).
		Model(&models.Category{}).
		Select("categories.name AS category, COUNT(listings.id) AS count").
		Joins("LEFT JOIN listings ON listings.category_id = categories.id AND listings.active = true").
		Group("categories.name").
		Order("categories.name").
		Scan(&out).Error
	return out, err
}

// FindOrCreateCategory resolves a (category, sub_category) name pair to a
// Category row, creating whichever levels don't exist yet, and returns the
// id to use as a Listing's CategoryID — the sub-category's id if given,
// otherwise the top-level category's id.
func (s *Store) FindOrCreateCategory(ctx context.Context, name, subName string) (uint, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Uncategorized"
	}

	top, err := s.findOrCreateCategoryLevel(ctx, name, nil, 0)
	if err != nil {
		return 0, err
	}

	subName = strings.TrimSpace(subName)
	if subName == "" {
		return top.ID, nil
	}

	sub, err := s.findOrCreateCategoryLevel(ctx, subName, &top.ID, top.Level+1)
	if err != nil {
		return 0, err
	}
	return sub.ID, nil
}

// findOrCreateCategoryLevel looks up a category by name + parent, creating it
// if missing. Matching is case-insensitive within the same parent.
func (s *Store) findOrCreateCategoryLevel(ctx context.Context, name string, parentID *uint, level int) (*models.Category, error) {
	q := s.db.WithContext(ctx).Where("LOWER(name) = LOWER(?)", name)
	if parentID == nil {
		q = q.Where("parent_id IS NULL")
	} else {
		q = q.Where("parent_id = ?", *parentID)
	}

	var cat models.Category
	err := q.First(&cat).Error
	if err == nil {
		return &cat, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	cat = models.Category{
		Name:     name,
		Slug:     toSlug(name),
		ParentID: parentID,
		Level:    level,
	}
	if err := s.db.WithContext(ctx).Create(&cat).Error; err != nil {
		return nil, err
	}
	return &cat, nil
}

func toSlug(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), " ", "-")
}

// ── Listings search ───────────────────────────────────────────────────────────

// SearchParams captures the filters the bot or console can pass.
type SearchParams struct {
	Query    string // free text matched against name/address/description/category/data
	Category string // exact category name filter (case-insensitive)
	Limit    int    // 0 -> default 20
}

// SearchListings finds active listings matching the params. Free text is
// tokenized: each word must appear in SOME searchable field (words AND-ed,
// fields OR-ed), with light plural stemming so "fans" matches "fan".
func (s *Store) SearchListings(ctx context.Context, p SearchParams) ([]models.Listing, error) {
	q := s.db.WithContext(ctx).
		Model(&models.Listing{}).
		Joins("LEFT JOIN categories ON categories.id = listings.category_id").
		Where("listings.active = true").
		Preload("Category")

	for _, tok := range tokenize(p.Query) {
		like := "%" + tok + "%"
		q = q.Where(
			s.db.Where("listings.name ILIKE ?", like).
				Or("listings.address ILIKE ?", like).
				Or("listings.description ILIKE ?", like).
				Or("categories.name ILIKE ?", like).
				Or("listings.data::text ILIKE ?", like),
		)
	}

	if c := strings.TrimSpace(p.Category); c != "" {
		q = q.Where("categories.name ILIKE ?", c)
	}

	limit := p.Limit
	if limit <= 0 {
		limit = 20
	}

	out := []models.Listing{}
	err := q.Order("listings.name").Limit(limit).Find(&out).Error
	return out, err
}

// ── Alerts (waitlist) ─────────────────────────────────────────────────────────

// CreateAlertRequest durably records a customer's "notify me when available"
// request, defaulting Status to "logged". Returns the new id.
func (s *Store) CreateAlertRequest(ctx context.Context, r models.AlertRequest) (uint, error) {
	if r.Status == "" {
		r.Status = "logged"
	}
	if err := s.db.WithContext(ctx).Create(&r).Error; err != nil {
		return 0, err
	}
	return r.ID, nil
}

// ── Contacts (who messaged the bot) ───────────────────────────────────────────

// IsStaffPhone reports whether the given +E.164 number belongs to a staff or
// owner account — the trusted check behind staff-only bot tools.
func (s *Store) IsStaffPhone(ctx context.Context, phone string) bool {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return false
	}
	var count int64
	s.db.WithContext(ctx).Model(&models.User{}).
		Where("phone = ? AND active = true", phone).
		Count(&count)
	return count > 0
}

// RecordContact upserts the contact for a phone number: first message creates
// the row, repeats bump the interaction count and last-seen. Staff/owner numbers
// are flagged so they aren't treated as customer leads. Non-fatal by design.
func (s *Store) RecordContact(ctx context.Context, phone, name, query string) {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return
	}
	now := time.Now()
	isStaff := s.IsStaffPhone(ctx, phone)

	var contact models.Contact
	err := s.db.WithContext(ctx).Where("phone = ?", phone).First(&contact).Error
	if err == gorm.ErrRecordNotFound {
		s.db.WithContext(ctx).Create(&models.Contact{
			Phone: phone, DisplayName: strings.TrimSpace(name), IsStaff: isStaff,
			Interactions: 1, LastQuery: strings.TrimSpace(query),
			FirstSeen: now, LastSeen: now,
		})
		return
	}
	if err != nil {
		return
	}
	updates := map[string]interface{}{
		"interactions": contact.Interactions + 1,
		"last_seen":    now,
		"is_staff":     isStaff,
	}
	if q := strings.TrimSpace(query); q != "" {
		updates["last_query"] = q
	}
	if n := strings.TrimSpace(name); n != "" {
		updates["display_name"] = n
	}
	s.db.WithContext(ctx).Model(&models.Contact{}).Where("id = ?", contact.ID).Updates(updates)
}

// RecentContacts returns the most recently active contacts, newest first. When
// `since` is non-zero, only contacts seen at/after that time are returned
// (powers the Today / last week / last month filters).
func (s *Store) RecentContacts(ctx context.Context, limit int, since time.Time) ([]models.Contact, error) {
	if limit <= 0 {
		limit = 100
	}
	q := s.db.WithContext(ctx).Order("last_seen DESC").Limit(limit)
	if !since.IsZero() {
		q = q.Where("last_seen >= ?", since)
	}
	out := []models.Contact{}
	err := q.Find(&out).Error
	return out, err
}

// CountContacts counts contacts seen at/after `since` (zero = all time).
func (s *Store) CountContacts(ctx context.Context, since time.Time) int64 {
	var n int64
	q := s.db.WithContext(ctx).Model(&models.Contact{})
	if !since.IsZero() {
		q = q.Where("last_seen >= ?", since)
	}
	q.Count(&n)
	return n
}

// ── Bot activity log ──────────────────────────────────────────────────────────

// LogActivity records one bot tool call for the owner's monitoring view.
func (s *Store) LogActivity(ctx context.Context, a models.BotActivity) error {
	return s.db.WithContext(ctx).Create(&a).Error
}

// ── Tokenization (ported from Signet) ─────────────────────────────────────────

// tokenize splits a free-text query into stemmed lowercase words.
func tokenize(query string) []string {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	out := []string{}
	for _, tok := range strings.Fields(q) {
		if stem := stemToken(tok); stem != "" {
			out = append(out, stem)
		}
	}
	return out
}

// stemToken lowercases a word and strips a trailing plural "s" so a customer's
// plural still matches the singular catalog text. Short words (<=3 chars) are
// left untouched to avoid over-stripping; "ss" endings are preserved.
func stemToken(tok string) string {
	t := strings.ToLower(strings.TrimSpace(tok))
	if len(t) > 3 && strings.HasSuffix(t, "s") && !strings.HasSuffix(t, "ss") {
		t = strings.TrimSuffix(t, "s")
	}
	return t
}
