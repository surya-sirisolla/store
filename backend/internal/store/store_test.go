package store

import (
	"context"
	"testing"

	"store/backend/internal/models"
	"store/backend/internal/testutil"
)

func TestStemToken(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Fans", "fan"},
		{"bulbs", "bulb"},
		{"LED", "led"},
		{"gas", "gas"},      // <=3 chars untouched
		{"glass", "glass"},  // "ss" preserved
		{"  Wire  ", "wire"},
	}
	for _, tt := range tests {
		if got := stemToken(tt.in); got != tt.want {
			t.Errorf("stemToken(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// seedListings creates a category with the given listings and returns category id.
func seedDirectory(t *testing.T, s *Store) {
	t.Helper()
	db := s.DB()
	lighting := models.Category{Name: "Lighting", Slug: "lighting"}
	fans := models.Category{Name: "Fans", Slug: "fans"}
	db.Create(&lighting)
	db.Create(&fans)

	rows := []models.Listing{
		{Name: "Philips LED Bulb 9W", CategoryID: lighting.ID, Phone: "111", Active: true, Description: "warm white"},
		{Name: "Havells Ceiling Fan", CategoryID: fans.ID, Phone: "222", Active: true, Address: "Aisle 3"},
		{Name: "Orient Table Fan", CategoryID: fans.ID, Phone: "333", Active: true},
		{Name: "Inactive Bulb", CategoryID: lighting.ID, Phone: "444", Active: true},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatalf("seed listing: %v", err)
		}
	}
	// GORM treats a zero-value bool as "unset" and applies default:true, so mark
	// the inactive listing with an explicit Update rather than at insert time.
	if err := db.Model(&rows[3]).Update("active", false).Error; err != nil {
		t.Fatalf("deactivate listing: %v", err)
	}
}

func TestSearchListings(t *testing.T) {
	s := New(testutil.NewDB(t))
	seedDirectory(t, s)
	ctx := context.Background()

	tests := []struct {
		name     string
		params   SearchParams
		wantLen  int
		wantName string // a name that must be present (optional)
	}{
		{"plural stems to singular", SearchParams{Query: "fans"}, 2, "Havells Ceiling Fan"},
		{"multi-word AND", SearchParams{Query: "led bulb"}, 1, "Philips LED Bulb 9W"},
		{"category-name match via join", SearchParams{Query: "lighting"}, 1, "Philips LED Bulb 9W"},
		{"category filter", SearchParams{Category: "Fans"}, 2, ""},
		{"excludes inactive", SearchParams{Query: "bulb"}, 1, "Philips LED Bulb 9W"},
		{"no match", SearchParams{Query: "refrigerator"}, 0, ""},
		{"empty query returns all active", SearchParams{}, 3, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.SearchListings(ctx, tt.params)
			if err != nil {
				t.Fatalf("SearchListings: %v", err)
			}
			if len(got) != tt.wantLen {
				names := make([]string, len(got))
				for i, l := range got {
					names[i] = l.Name
				}
				t.Fatalf("len = %d, want %d (%v)", len(got), tt.wantLen, names)
			}
			if tt.wantName != "" {
				found := false
				for _, l := range got {
					if l.Name == tt.wantName {
						found = true
					}
				}
				if !found {
					t.Errorf("expected %q in results", tt.wantName)
				}
			}
		})
	}
}

func TestSearchListings_PreloadsCategory(t *testing.T) {
	s := New(testutil.NewDB(t))
	seedDirectory(t, s)
	got, _ := s.SearchListings(context.Background(), SearchParams{Query: "ceiling"})
	if len(got) != 1 || got[0].Category.Name != "Fans" {
		t.Fatalf("expected category preloaded as Fans, got %+v", got)
	}
}

func TestBusinessProfile_Upsert(t *testing.T) {
	s := New(testutil.NewDB(t))
	ctx := context.Background()

	if p, _ := s.GetBusinessProfile(ctx); p != nil {
		t.Fatal("expected nil profile initially")
	}
	if err := s.UpsertBusinessProfile(ctx, &models.BusinessProfile{
		Name: "Adonai Electronics", City: "Guntur", Phones: models.StringSlice{"999"},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Second upsert must update the same single row, not insert a new one.
	if err := s.UpsertBusinessProfile(ctx, &models.BusinessProfile{Name: "Adonai Electronics v2"}); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	var count int64
	s.DB().Model(&models.BusinessProfile{}).Count(&count)
	if count != 1 {
		t.Errorf("profile rows = %d, want 1", count)
	}
	p, _ := s.GetBusinessProfile(ctx)
	if p == nil || p.Name != "Adonai Electronics v2" {
		t.Errorf("profile = %+v, want name 'Adonai Electronics v2'", p)
	}
}

func TestAlertAndActivity(t *testing.T) {
	s := New(testutil.NewDB(t))
	ctx := context.Background()

	id, err := s.CreateAlertRequest(ctx, models.AlertRequest{
		CustomerName: "Ravi", CustomerPhone: "98765", ItemQuery: "inverter battery",
		Availability: "not_carried",
	})
	if err != nil || id == 0 {
		t.Fatalf("CreateAlertRequest: id=%d err=%v", id, err)
	}
	var ar models.AlertRequest
	s.DB().First(&ar, id)
	if ar.Status != "logged" {
		t.Errorf("status = %q, want logged", ar.Status)
	}

	if err := s.LogActivity(ctx, models.BotActivity{Tool: "search_listings", Query: "fan", ResultSummary: "2 results"}); err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	var n int64
	s.DB().Model(&models.BotActivity{}).Count(&n)
	if n != 1 {
		t.Errorf("activity rows = %d, want 1", n)
	}
}

func TestCategoryCounts(t *testing.T) {
	s := New(testutil.NewDB(t))
	seedDirectory(t, s)
	counts, err := s.CategoryCounts(context.Background())
	if err != nil {
		t.Fatalf("CategoryCounts: %v", err)
	}
	got := map[string]int{}
	for _, c := range counts {
		got[c.Category] = c.Count
	}
	// Lighting has 1 active (inactive bulb excluded), Fans has 2.
	if got["Lighting"] != 1 || got["Fans"] != 2 {
		t.Errorf("counts = %v, want Lighting:1 Fans:2", got)
	}
}

// ── Admin credential (the console's single login) ─────────────────────────────

func TestEnsureAdmin_SeedsOnceAndRenames(t *testing.T) {
	s := New(testutil.NewDB(t))
	ctx := context.Background()

	if a, _ := s.GetAdmin(ctx); a != nil {
		t.Fatal("expected no admin before seeding")
	}

	created, err := s.EnsureAdmin(ctx, "admin", "hash-one")
	if err != nil || !created {
		t.Fatalf("first EnsureAdmin: created=%v err=%v", created, err)
	}

	// Re-seeding must not create a second admin, and must NOT overwrite the
	// password — the env value is ignored once the credential exists, so a
	// password rotated in the console survives a restart.
	created, err = s.EnsureAdmin(ctx, "admin", "hash-two")
	if err != nil || created {
		t.Fatalf("second EnsureAdmin: created=%v err=%v", created, err)
	}
	var count int64
	s.DB().Model(&models.AuthCredential{}).Count(&count)
	if count != 1 {
		t.Fatalf("admin rows = %d, want 1", count)
	}
	a, _ := s.GetAdmin(ctx)
	if a.PasswordHash != "hash-one" {
		t.Errorf("password hash = %q, want the original 'hash-one'", a.PasswordHash)
	}

	// A changed ADMIN_USER renames the existing admin in place.
	if _, err := s.EnsureAdmin(ctx, "owner", "hash-three"); err != nil {
		t.Fatalf("rename EnsureAdmin: %v", err)
	}
	a, _ = s.GetAdmin(ctx)
	if a.Username != "owner" {
		t.Errorf("username = %q, want 'owner'", a.Username)
	}
	if a.PasswordHash != "hash-one" {
		t.Errorf("rename changed the password hash to %q", a.PasswordHash)
	}
	s.DB().Model(&models.AuthCredential{}).Count(&count)
	if count != 1 {
		t.Errorf("admin rows after rename = %d, want 1", count)
	}
}

func TestSetAdminPassword(t *testing.T) {
	s := New(testutil.NewDB(t))
	ctx := context.Background()

	if _, err := s.EnsureAdmin(ctx, "admin", "old"); err != nil {
		t.Fatalf("EnsureAdmin: %v", err)
	}
	if err := s.SetAdminPassword(ctx, "new"); err != nil {
		t.Fatalf("SetAdminPassword: %v", err)
	}
	a, _ := s.GetAdmin(ctx)
	if a.PasswordHash != "new" {
		t.Errorf("password hash = %q, want 'new'", a.PasswordHash)
	}
}

// TestBotSeesCatalog is the regression test for the "no catalog present" bug.
// The MCP server (cmd/mcp) that backs the WhatsApp bot builds its store with a
// plain store.New — exactly as constructed here. It used to bind to one
// "primary business" at startup and answered from that tenant's catalog, so a
// catalog entered under any other tenant was invisible to the bot. With
// tenancy gone there is nothing to bind to: whatever the console writes, the
// bot reads.
func TestBotSeesCatalog(t *testing.T) {
	db := testutil.NewDB(t)
	ctx := context.Background()

	// What the console writes.
	console := New(db)
	catID, err := console.FindOrCreateCategory(ctx, "Refrigerators", "")
	if err != nil {
		t.Fatalf("FindOrCreateCategory: %v", err)
	}
	qty := 10
	if err := db.Create(&models.Listing{
		CategoryID: catID, Name: "Samsung Refrigerator", Quantity: &qty, Active: true,
	}).Error; err != nil {
		t.Fatalf("create listing: %v", err)
	}

	// What the bot reads — a separately constructed store, as cmd/mcp does.
	bot := New(db)

	items, err := bot.SearchListings(ctx, SearchParams{Query: "refrigerator"})
	if err != nil {
		t.Fatalf("SearchListings: %v", err)
	}
	if len(items) != 1 || items[0].Name != "Samsung Refrigerator" {
		t.Fatalf("bot search returned %d items (%+v), want the Samsung Refrigerator", len(items), items)
	}

	counts, err := bot.CategoryCounts(ctx)
	if err != nil {
		t.Fatalf("CategoryCounts: %v", err)
	}
	if len(counts) != 1 || counts[0].Category != "Refrigerators" || counts[0].Count != 1 {
		t.Fatalf("bot categories = %+v, want Refrigerators:1", counts)
	}

	// And the profile the bot quotes for contact details.
	if err := console.UpsertBusinessProfile(ctx, &models.BusinessProfile{Name: "Bajaj"}); err != nil {
		t.Fatalf("UpsertBusinessProfile: %v", err)
	}
	p, err := bot.GetBusinessProfile(ctx)
	if err != nil || p == nil || p.Name != "Bajaj" {
		t.Fatalf("bot profile = %+v (err %v), want Bajaj", p, err)
	}
}
