package handlers

import (
	"net/http"
	"time"

	"store/backend/internal/models"
	"store/backend/internal/store"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AlertsHandler serves the console's Alerts section — the customer waitlist the
// WhatsApp bot captures, plus the "ready to notify" alerts the matcher raises
// when a waitlisted item comes back in stock.
type AlertsHandler struct {
	db *gorm.DB
	st *store.Store
}

func NewAlertsHandler(db *gorm.DB, st *store.Store) *AlertsHandler {
	return &AlertsHandler{db: db, st: st}
}

// List returns alerts, optionally filtered by ?status=, newest first. For raised
// ("ready") alerts it attaches the matched listing so the console can show the
// item's current stock and price.
func (h *AlertsHandler) List(c *gin.Context) {
	items := []models.AlertRequest{}
	q := h.db.Order("CASE status WHEN 'ready' THEN 0 WHEN 'logged' THEN 1 ELSE 2 END").
		Order("created_at DESC")
	if status := c.Query("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	q.Limit(300).Find(&items)
	h.attachListings(items)
	c.JSON(http.StatusOK, items)
}

// attachListings fills MatchedListing for any alert that links one, in a single
// query.
func (h *AlertsHandler) attachListings(items []models.AlertRequest) {
	ids := []uint{}
	for _, a := range items {
		if a.ListingID != nil {
			ids = append(ids, *a.ListingID)
		}
	}
	if len(ids) == 0 {
		return
	}
	listings := []models.Listing{}
	h.db.Preload("Category").Where("id IN ?", ids).Find(&listings)
	byID := map[uint]models.Listing{}
	for i := range listings {
		byID[listings[i].ID] = listings[i]
	}
	for i := range items {
		if items[i].ListingID != nil {
			if l, ok := byID[*items[i].ListingID]; ok {
				lc := l
				items[i].MatchedListing = &lc
			}
		}
	}
}

// Counts returns how many alerts sit in each state, for badges.
func (h *AlertsHandler) Counts(c *gin.Context) {
	var logged, ready, notified, dismissed int64
	h.db.Model(&models.AlertRequest{}).Where("status = ?", "logged").Count(&logged)
	h.db.Model(&models.AlertRequest{}).Where("status = ?", "ready").Count(&ready)
	h.db.Model(&models.AlertRequest{}).Where("status = ?", "notified").Count(&notified)
	h.db.Model(&models.AlertRequest{}).Where("status = ?", "dismissed").Count(&dismissed)
	c.JSON(http.StatusOK, gin.H{
		"logged": logged, "ready": ready, "notified": notified, "dismissed": dismissed,
		"open": logged + ready,
	})
}

// SetStatus moves an alert to a new state (typically ready → notified, or
// anything → dismissed). Setting "notified" stamps notified_at.
func (h *AlertsHandler) SetStatus(c *gin.Context) {
	var in struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	switch in.Status {
	case "logged", "ready", "notified", "dismissed":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	updates := map[string]interface{}{"status": in.Status}
	if in.Status == "notified" {
		updates["notified_at"] = time.Now()
	}
	res := h.db.Model(&models.AlertRequest{}).Where("id = ?", c.Param("id")).Updates(updates)
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "alert not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

// Recheck re-runs the matcher on demand, raising any logged alert whose item is
// now in stock. Handy right after the owner adds stock manually.
func (h *AlertsHandler) Recheck(c *gin.Context) {
	raised := h.st.RaiseReadyAlerts(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"raised": raised})
}
