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
