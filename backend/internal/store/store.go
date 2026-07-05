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

// ── Console auth ───────────────────────────────────────────────────────────────

// GetOwnerPasswordHash returns the stored bcrypt hash for the console login, or
// "" if none has been set yet.
func (s *Store) GetOwnerPasswordHash(ctx context.Context) (string, error) {
	var a models.AuthCredential
	err := s.db.WithContext(ctx).First(&a).Error
	if err == gorm.ErrRecordNotFound {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return a.PasswordHash, nil
}

// SetOwnerPassword upserts the single console-login credential (id forced to 1).
func (s *Store) SetOwnerPassword(ctx context.Context, hash string) error {
	a := models.AuthCredential{ID: 1, PasswordHash: hash}
	return s.db.WithContext(ctx).Save(&a).Error
}

// ── Business locations (godowns) ───────────────────────────────────────────────

// ListBusinessLocations returns every physical location, ordered by name.
func (s *Store) ListBusinessLocations(ctx context.Context) ([]models.BusinessLocation, error) {
	locs := []models.BusinessLocation{}
	err := s.db.WithContext(ctx).Order("name").Find(&locs).Error
	return locs, err
}

// GetBusinessLocation returns one location by id, or nil if it doesn't exist.
func (s *Store) GetBusinessLocation(ctx context.Context, id uint) (*models.BusinessLocation, error) {
	var l models.BusinessLocation
	err := s.db.WithContext(ctx).First(&l, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// UpdateBusinessLocation saves owner edits (name + full address + phone + active)
// for one location without touching its SourceID, so re-syncs still match it.
func (s *Store) UpdateBusinessLocation(ctx context.Context, id uint, in models.BusinessLocation) error {
	return s.db.WithContext(ctx).Model(&models.BusinessLocation{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"name":    in.Name,
			"address": in.Address,
			"area":    in.Area,
			"city":    in.City,
			"state":   in.State,
			"pincode": in.Pincode,
			"phone":   in.Phone,
			"active":  in.Active,
		}).Error
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
	// InStock, when true, hides items that are out of stock — quantity 0 or
	// negative. Items with an unknown quantity (nil, e.g. a service) are kept.
	// Used for customer-facing searches so the bot never offers something the
	// business can't currently supply.
	InStock bool
	// Stock is a console-side quantity filter: "available" (>0), "out" (=0),
	// "negative" (<0), or "unknown" (no quantity). Empty means no filter.
	Stock string
}

// StockCondition maps a stock filter value to a SQL predicate on the listings
// quantity column, or "" for no filter / an unrecognized value. The returned
// string is a fixed constant (never interpolated user input), so it's safe to
// pass straight to a WHERE clause.
func StockCondition(stock string) string {
	switch strings.TrimSpace(stock) {
	case "available":
		return "quantity > 0"
	case "out":
		return "quantity = 0"
	case "negative":
		return "quantity < 0"
	case "unknown":
		return "quantity IS NULL"
	default:
		return ""
	}
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
		// Short abbreviations like "ac", "dc", "led" are matched on WORD boundaries
		// so "ac" hits "SPLIT AC"/"Tower Ac" but not "ACCESSORIES"/"black"/"pack".
		// Longer tokens keep loose substring matching for partial-word matches.
		if isShortWordToken(tok) {
			pat := `\y` + tok + `\y`
			q = q.Where(
				s.db.Where("listings.name ~* ?", pat).
					Or("listings.address ~* ?", pat).
					Or("listings.description ~* ?", pat).
					Or("categories.name ~* ?", pat).
					Or("listings.data::text ~* ?", pat),
			)
			continue
		}
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

	if p.InStock {
		// nil quantity = service/unknown, keep it; 0 or negative = out of stock.
		q = q.Where("listings.quantity IS NULL OR listings.quantity > 0")
	}

	if cond := StockCondition(p.Stock); cond != "" {
		q = q.Where("listings." + cond)
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
// request, defaulting Status to "logged". It de-duplicates: if the same phone
// already has an open (logged/ready) request for the same item, that existing
// row is refreshed instead of inserting a duplicate, and its id is returned.
func (s *Store) CreateAlertRequest(ctx context.Context, r models.AlertRequest) (uint, error) {
	if r.Status == "" {
		r.Status = "logged"
	}
	if r.Source == "" {
		r.Source = "customer_asked"
	}

	phone := strings.TrimSpace(r.CustomerPhone)
	item := strings.TrimSpace(r.ItemQuery)
	if phone != "" && item != "" {
		var existing models.AlertRequest
		err := s.db.WithContext(ctx).
			Where("customer_phone = ? AND LOWER(item_query) = LOWER(?) AND status IN ?",
				phone, item, []string{"logged", "ready"}).
			First(&existing).Error
		if err == nil {
			// Refresh the name/category if the new call carries better info.
			updates := map[string]interface{}{}
			if n := strings.TrimSpace(r.CustomerName); n != "" && n != existing.CustomerName {
				updates["customer_name"] = n
			}
			if cat := strings.TrimSpace(r.Category); cat != "" {
				updates["category"] = cat
			}
			if len(updates) > 0 {
				s.db.WithContext(ctx).Model(&existing).Updates(updates)
			}
			return existing.ID, nil
		}
		if err != gorm.ErrRecordNotFound {
			return 0, err
		}
	}

	if err := s.db.WithContext(ctx).Create(&r).Error; err != nil {
		return 0, err
	}
	return r.ID, nil
}

// FindAvailableMatch returns the first active listing that satisfies the free-text
// query and is actually available — quantity unknown (nil, e.g. a service) or
// greater than zero. Used to decide whether a waitlisted item is back.
func (s *Store) FindAvailableMatch(ctx context.Context, query string) (*models.Listing, bool) {
	if strings.TrimSpace(query) == "" {
		return nil, false
	}
	items, err := s.SearchListings(ctx, SearchParams{Query: query, Limit: 10})
	if err != nil {
		return nil, false
	}
	for i := range items {
		if items[i].Quantity == nil || *items[i].Quantity > 0 {
			return &items[i], true
		}
	}
	return nil, false
}

// RaiseReadyAlerts scans every still-logged alert and, for any whose item is now
// available, flips it to "ready" and links the matching listing. Returns how many
// it raised. Called after a stock sync and after a listing is created/restocked,
// so the owner is prompted to notify the customer. Cheap: the waitlist is small.
func (s *Store) RaiseReadyAlerts(ctx context.Context) int {
	var pending []models.AlertRequest
	if err := s.db.WithContext(ctx).Where("status = ?", "logged").Find(&pending).Error; err != nil {
		return 0
	}
	now := time.Now()
	raised := 0
	for _, a := range pending {
		match, ok := s.FindAvailableMatch(ctx, a.ItemQuery)
		if !ok {
			continue
		}
		updates := map[string]interface{}{
			"status":     "ready",
			"listing_id": match.ID,
			"ready_at":   now,
		}
		if err := s.db.WithContext(ctx).Model(&models.AlertRequest{}).
			Where("id = ?", a.ID).Updates(updates).Error; err == nil {
			raised++
		}
	}
	return raised
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

// SetContactOptOut flips a contact's reminder opt-out by phone. Used when a
// customer replies "STOP" (out=true) or "START" (out=false). No-op if unknown.
func (s *Store) SetContactOptOut(ctx context.Context, phone string, out bool) {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return
	}
	s.db.WithContext(ctx).Model(&models.Contact{}).
		Where("phone = ?", phone).Update("opted_out", out)
}

// BroadcastRecipients returns the contacts eligible for a reminder broadcast:
// opted-in numbers, optionally limited to those seen within sinceDays (0 = all)
// and optionally excluding staff. Newest-active first.
func (s *Store) BroadcastRecipients(ctx context.Context, sinceDays int, includeStaff bool) ([]models.Contact, error) {
	q := s.db.WithContext(ctx).Where("opted_out = ?", false)
	if !includeStaff {
		q = q.Where("is_staff = ?", false)
	}
	if sinceDays > 0 {
		q = q.Where("last_seen >= ?", time.Now().AddDate(0, 0, -sinceDays))
	}
	out := []models.Contact{}
	err := q.Order("last_seen DESC").Find(&out).Error
	return out, err
}

// FeaturedListings returns active, in-stock listings the owner flagged as
// featured/offers, for composing a reminder message. limit <= 0 → 5.
func (s *Store) FeaturedListings(ctx context.Context, limit int) ([]models.Listing, error) {
	if limit <= 0 {
		limit = 5
	}
	out := []models.Listing{}
	err := s.db.WithContext(ctx).
		Where("active = true AND featured = true").
		Where("quantity IS NULL OR quantity > 0").
		Order("updated_at DESC").
		Limit(limit).
		Find(&out).Error
	return out, err
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

// isShortWordToken reports whether a token is a short (<=3 char) purely
// alphanumeric word — the case where loose substring matching produces noise, so
// we match it on word boundaries instead. Alphanumeric-only keeps the value safe
// to drop straight into a Postgres regex (no metacharacters to escape).
func isShortWordToken(s string) bool {
	if len(s) == 0 || len(s) > 3 {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
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
