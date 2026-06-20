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
// general questions about (address, timings, contact, services). One row only.
type BusinessProfile struct {
	ID        uint        `gorm:"primaryKey" json:"id"`
	Name      string      `gorm:"not null" json:"name"`
	About     string      `json:"about"`
	Address   string      `json:"address"`
	Area      string      `json:"area"`
	City      string      `json:"city"`
	State     string      `json:"state"`
	Pincode   string      `json:"pincode"`
	Phones    StringSlice `gorm:"type:jsonb" json:"phones"`
	WhatsApp  string      `json:"whatsapp"`
	Hours     string      `json:"hours"`
	Services  StringSlice `gorm:"type:jsonb" json:"services"`
	UpdatedAt time.Time   `json:"updated_at"`
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
	ID          uint      `gorm:"primaryKey" json:"id"`
	CategoryID  uint      `gorm:"not null;index" json:"category_id"`
	Category    Category  `json:"category"`
	Name        string    `gorm:"not null" json:"name"`
	Phone       string    `json:"phone"`
	Address     string    `json:"address"`
	Description string    `json:"description"`
	Quantity    *int      `json:"quantity,omitempty"`
	Price       *float64  `json:"price,omitempty"`
	Data        JSONB     `gorm:"type:jsonb" json:"data"`
	Active      bool      `gorm:"default:true" json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AlertRequest is the waitlist: a customer asking (via the bot) to be notified
// when something the business can't currently supply becomes available.
// Ported from Signet's StockRequest.
type AlertRequest struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	CustomerName  string    `json:"customer_name"`
	CustomerPhone string    `json:"customer_phone"`
	ItemQuery     string    `json:"item_query"`                     // what the customer asked for, verbatim
	Category      string    `json:"category"`                       // category if known
	Availability  string    `json:"availability"`                   // "out_of_stock" or "not_carried"
	Status        string    `gorm:"default:'logged'" json:"status"` // logged | notified
	CreatedAt     time.Time `gorm:"index" json:"created_at"`
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
	ID           uint      `gorm:"primaryKey" json:"id"`
	Phone        string    `gorm:"uniqueIndex;not null" json:"phone"` // +E.164
	DisplayName  string    `json:"display_name"`
	IsStaff      bool      `gorm:"default:false" json:"is_staff"`
	Interactions int       `gorm:"default:0" json:"interactions"`
	LastQuery    string    `json:"last_query"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `gorm:"index" json:"last_seen"`
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
