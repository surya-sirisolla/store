package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

type Role string

const (
	// RoleOwner is the single business owner — full access.
	RoleOwner Role = "owner"
	// RoleStaff is an optional data-entry helper the owner creates.
	RoleStaff Role = "staff"
)

// JSONB stores arbitrary JSON in a Postgres jsonb column.
type JSONB map[string]interface{}

func (j JSONB) Value() (driver.Value, error) {
	b, err := json.Marshal(j)
	return string(b), err
}

func (j *JSONB) Scan(value interface{}) error {
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	case nil:
		*j = JSONB{}
		return nil
	default:
		return errors.New("unsupported type for JSONB")
	}
	return json.Unmarshal(bytes, j)
}

// StringSlice stores a []string in a jsonb column (phones, services, etc.).
type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
	b, err := json.Marshal(s)
	return string(b), err
}

func (s *StringSlice) Scan(value interface{}) error {
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	case nil:
		*s = StringSlice{}
		return nil
	default:
		return errors.New("unsupported type for StringSlice")
	}
	return json.Unmarshal(bytes, s)
}

// User is the owner or a staff member. There's no console login — a User
// record exists only to register someone's WhatsApp number so the bot can
// recognize them for permission-scoped replies (e.g. staff-only tools).
// Single-tenant: every user shares the one business's data, so there is no
// tenant/admin linkage.
type User struct {
	ID    uint   `gorm:"primaryKey" json:"id"`
	Name  string `gorm:"not null" json:"name"`
	Email string `gorm:"uniqueIndex;not null" json:"email"`
	Role  Role   `gorm:"type:varchar(20);not null;default:'staff'" json:"role"`
	// Phone is the person's WhatsApp number in E.164 form (e.g.
	// +919876543210). It identifies them to the bot for permission-scoped
	// replies and lets the business reach them with alerts.
	Phone     string    `gorm:"index" json:"phone"`
	Active    bool      `gorm:"default:true" json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BusinessProfile is the single storefront record the WhatsApp agent answers
// general questions about. Kept deliberately minimal — identity + contact only
// (name, email, mobile numbers, opening hours). Per-branch street addresses live
// on BusinessLocation, not here. One row only.
//
// The address/whatsapp/services/about columns predate the move to per-location
// addresses; they're retained so AutoMigrate doesn't fail on upgraded installs,
// but the console and bot no longer use them.
type BusinessProfile struct {
	ID    uint   `gorm:"primaryKey" json:"id"`
	Name  string `gorm:"not null" json:"name"`
	Email string `json:"email"`
	// Phones are the business's mobile/phone contact numbers the bot shares.
	Phones StringSlice `gorm:"type:jsonb" json:"phones"`
	Hours  string      `json:"hours"`

	// Deprecated (no longer surfaced in the console/bot; see BusinessLocation).
	About     string      `json:"about,omitempty"`
	Address   string      `json:"address,omitempty"`
	Area      string      `json:"area,omitempty"`
	City      string      `json:"city,omitempty"`
	State     string      `json:"state,omitempty"`
	Pincode   string      `json:"pincode,omitempty"`
	WhatsApp  string      `json:"whatsapp,omitempty"`
	Services  StringSlice `gorm:"type:jsonb" json:"services,omitempty"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// BusinessLocation is one physical branch/warehouse of the business — sourced
// from Livekeeping's godowns (getGodownsSummary). Each Livekeeping godown maps
// to one row so the owner can attach a real, editable street address to every
// location the WhatsApp bot might mention. Name comes from Livekeeping; Address
// is owner-entered and preserved across re-syncs.
type BusinessLocation struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `gorm:"not null" json:"name"` // godownName from Livekeeping
	// Structured street address, all owner-entered. Blank on first sync and never
	// overwritten by later syncs, so owner edits survive re-syncs. The bot reads
	// these to tell customers which branch stocks an item and where it is.
	Address string `json:"address"` // street / building line
	Area    string `json:"area"`
	City    string `json:"city"`
	State   string `json:"state"`
	Pincode string `json:"pincode"`
	Phone   string `json:"phone"` // optional per-branch contact number
	// SourceID is Livekeeping's stable godownGuid. Indexed so a re-sync upserts
	// by it instead of creating duplicates. Empty for manually-added locations.
	SourceID  string    `gorm:"index" json:"source_id,omitempty"`
	Active    bool      `gorm:"default:true" json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Category supports multi-level nesting via ParentID. Single-tenant: no admin scope.
type Category struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Name      string     `gorm:"not null" json:"name"`
	Slug      string     `gorm:"not null" json:"slug"`
	ParentID  *uint      `json:"parent_id,omitempty"`
	Parent    *Category  `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
	Children  []Category `gorm:"foreignKey:ParentID" json:"children,omitempty"`
	Level     int        `gorm:"default:0" json:"level"`
	CreatedAt time.Time  `json:"created_at"`
}

// Listing is one directory entry the bot searches. Extra fields live in Data.
type Listing struct {
	ID          uint     `gorm:"primaryKey" json:"id"`
	CategoryID  uint     `gorm:"not null;index" json:"category_id"`
	Category    Category `json:"category"`
	Name        string   `gorm:"not null" json:"name"`
	Phone       string   `json:"phone"`
	Address     string   `json:"address"`
	Description string   `json:"description"`
	Quantity    *int     `json:"quantity,omitempty"`
	Price       *float64 `json:"price,omitempty"`
	// HSNCode and Unit come from external inventory syncs (e.g. Livekeeping)
	// and are surfaced directly in the console; other synced fields live in Data.
	HSNCode string `json:"hsn_code,omitempty"`
	Unit    string `json:"unit,omitempty"`
	// SourceID is the external system's stable id for this item (Livekeeping
	// guid). Empty for manually-created listings. Indexed so a re-sync can
	// upsert by it instead of creating duplicates.
	SourceID string `gorm:"index" json:"source_id,omitempty"`
	// Featured marks an item as an offer/highlight the owner wants to push in
	// WhatsApp reminder broadcasts; OfferText is the optional deal line shown with
	// it (e.g. "10% off this week").
	Featured  bool      `gorm:"default:false;index" json:"featured"`
	OfferText string    `json:"offer_text,omitempty"`
	Data      JSONB     `gorm:"type:jsonb" json:"data"`
	Active    bool      `gorm:"default:true" json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AlertRequest is the waitlist: a customer asking (via the bot) to be notified
// when something the business can't currently supply becomes available.
// Ported from Signet's StockRequest.
//
// Lifecycle: logged → ready → notified, or → dismissed at any point.
//   - logged:    captured from the bot, item still unavailable.
//   - ready:     a matching listing is now in stock (auto-raised by the matcher);
//     this is the owner's cue to reach out.
//   - notified:  the owner has messaged the customer.
//   - dismissed: the owner closed it without notifying.
type AlertRequest struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	CustomerName  string `json:"customer_name"`
	CustomerPhone string `json:"customer_phone"`
	ItemQuery     string `json:"item_query"`   // what the customer asked for, verbatim
	Category      string `json:"category"`     // category if known
	Availability  string `json:"availability"` // "out_of_stock" or "not_carried"
	// Source records who initiated the alert: "customer_asked" (they said notify
	// me) or "bot_offered" (the bot proactively offered). Purely informational.
	Source string `gorm:"type:varchar(20);default:'customer_asked'" json:"source"`
	Status string `gorm:"type:varchar(20);default:'logged';index" json:"status"`

	// Set when the matcher raises the alert: the listing that came back in stock.
	ListingID  *uint      `json:"listing_id,omitempty"`
	ReadyAt    *time.Time `json:"ready_at,omitempty"`
	NotifiedAt *time.Time `json:"notified_at,omitempty"`

	// MatchedListing is populated on read (not stored) so the console can show the
	// current stock/price of a raised alert's item.
	MatchedListing *Listing `gorm:"-" json:"matched_listing,omitempty"`

	CreatedAt time.Time `gorm:"index" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BotActivity logs each MCP tool call the agent makes, so the owner can monitor
// what the WhatsApp bot is doing from the console.
type BotActivity struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Tool          string    `gorm:"index" json:"tool"` // search_listings | get_business_info | ...
	Query         string    `json:"query"`             // the user's query / key args
	ResultSummary string    `json:"result_summary"`    // e.g. "12 results", "alert logged"
	CustomerPhone string    `json:"customer_phone,omitempty"`
	CreatedAt     time.Time `gorm:"index" json:"created_at"`
}

// Contact is one WhatsApp number that has messaged the bot. Repeat messages
// from the same number update this row rather than creating duplicates, so the
// owner sees a real list of who has tried to reach the business. IsStaff is set
// when the number matches a staff/owner User.Phone.
type Contact struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Phone       string `gorm:"uniqueIndex;not null" json:"phone"` // +E.164
	DisplayName string `json:"display_name"`
	IsStaff     bool   `gorm:"default:false" json:"is_staff"`
	// OptedOut is set when the customer replies "STOP" — they're then excluded
	// from all reminder broadcasts until they opt back in ("START").
	OptedOut     bool      `gorm:"default:false" json:"opted_out"`
	Interactions int       `gorm:"default:0" json:"interactions"`
	LastQuery    string    `json:"last_query"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `gorm:"index" json:"last_seen"`
}

// ScheduledJob is one automated background task the owner manages from the
// console's Automation section (currently just the Livekeeping stock sync).
// There's one row per job Key; an in-process scheduler runs enabled jobs on
// their schedule and records the outcome here so the console can show it.
type ScheduledJob struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Key  string `gorm:"uniqueIndex;not null" json:"key"` // e.g. "livekeeping_sync"
	Name string `json:"name"`

	// Type is "system" (built-in jobs the owner can only schedule) or "broadcast"
	// (owner-created reminder jobs with a recipient list + message config).
	// Deletable is true only for owner-created jobs. Config holds broadcast
	// settings (recipients, message options, throttle/cap) — empty for system jobs.
	Type      string `gorm:"type:varchar(20);default:'system'" json:"type"`
	Deletable bool   `gorm:"default:false" json:"deletable"`
	Config    JSONB  `gorm:"type:jsonb" json:"config"`

	// Schedule. Kind is "manual" | "interval" | "daily". For "interval",
	// IntervalHours is the gap between runs; for "daily", DailyTime is "HH:MM"
	// (24h, UTC). "manual" (or Enabled=false) never auto-runs.
	Enabled       bool   `gorm:"default:false" json:"enabled"`
	ScheduleKind  string `gorm:"type:varchar(20);default:'manual'" json:"schedule_kind"`
	IntervalHours int    `gorm:"default:6" json:"interval_hours"`
	DailyTime     string `gorm:"type:varchar(5);default:'02:00'" json:"daily_time"`

	// Run bookkeeping, updated by the runner. LastResult holds the job's own
	// summary payload (for the sync: total/created/updated/deleted/errors).
	LastStatus  string     `gorm:"type:varchar(20);default:'idle'" json:"last_status"` // idle|running|success|failed
	LastMessage string     `json:"last_message"`
	LastResult  JSONB      `gorm:"type:jsonb" json:"last_result"`
	LastRunAt   *time.Time `json:"last_run_at,omitempty"`
	NextRunAt   *time.Time `json:"next_run_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BulkUploadJob tracks Excel import jobs.
type BulkUploadJob struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	FileName    string    `json:"file_name"`
	Status      string    `gorm:"default:'pending'" json:"status"` // pending|processing|done|failed
	TotalRows   int       `json:"total_rows"`
	Inserted    int       `json:"inserted"`
	Errors      int       `json:"errors"`
	ErrorDetail string    `gorm:"type:text" json:"error_detail,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
