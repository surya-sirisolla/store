package handlers

import (
	"net/http"
	"time"

	"store/backend/internal/models"

	"github.com/gin-gonic/gin"
)

// Stats returns the headline counts for the console dashboard.
func (h *ConsoleHandler) Stats(c *gin.Context) {
	var listings, categories, staff, interactions int64
	tdb := h.db
	tdb.Model(&models.Listing{}).Count(&listings)
	tdb.Model(&models.Category{}).Count(&categories)
	tdb.Model(&models.User{}).Count(&staff)
	tdb.Model(&models.BotActivity{}).Count(&interactions)
	c.JSON(http.StatusOK, gin.H{
		"listings":          listings,
		"categories":        categories,
		"staff":             staff,
		"bot_interactions":  interactions,
	})
}

// BotStats returns the headline numbers for the Bot Monitor page.
func (h *ConsoleHandler) BotStats(c *gin.Context) {
	startOfDay := time.Now().Truncate(24 * time.Hour)

	var totalQueries, queriesToday, searches, pendingAlerts, uniqueContacts, contactsToday int64
	tdb := h.db
	tdb.Model(&models.BotActivity{}).Count(&totalQueries)
	tdb.Model(&models.BotActivity{}).Where("created_at >= ?", startOfDay).Count(&queriesToday)
	tdb.Model(&models.BotActivity{}).Where("tool = ?", "search_listings").Count(&searches)
	// "Pending" = anything not yet actioned: still logged or raised-and-waiting.
	tdb.Model(&models.AlertRequest{}).Where("status IN ?", []string{"logged", "ready"}).Count(&pendingAlerts)
	var readyAlerts int64
	tdb.Model(&models.AlertRequest{}).Where("status = ?", "ready").Count(&readyAlerts)
	tdb.Model(&models.Contact{}).Count(&uniqueContacts)
	tdb.Model(&models.Contact{}).Where("last_seen >= ?", startOfDay).Count(&contactsToday)

	c.JSON(http.StatusOK, gin.H{
		"total_interactions": totalQueries,
		"interactions_today": queriesToday,
		"searches":           searches,
		"pending_alerts":     pendingAlerts,
		"ready_alerts":       readyAlerts,
		"unique_contacts":    uniqueContacts,
		"contacts_today":     contactsToday,
	})
}

// rangeSince converts a ?range= filter into a cutoff time. Empty/"all" → zero
// (no filter). Supports today | week | month.
func rangeSince(r string) time.Time {
	now := time.Now()
	switch r {
	case "today":
		return now.Truncate(24 * time.Hour)
	case "week":
		return now.AddDate(0, 0, -7)
	case "month":
		return now.AddDate(0, -1, 0)
	default:
		return time.Time{}
	}
}

// BotContacts returns the people who have messaged the bot, most recent first,
// filtered by ?range=today|week|month|all (default all).
func (h *ConsoleHandler) BotContacts(c *gin.Context) {
	since := rangeSince(c.Query("range"))
	items, err := h.st.RecentContacts(c.Request.Context(), 200, since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"contacts": items, "count": len(items)})
}

// BotActivityFeed returns the most recent bot tool calls.
func (h *ConsoleHandler) BotActivityFeed(c *gin.Context) {
	items := []models.BotActivity{}
	h.db.Order("created_at DESC").Limit(100).Find(&items)
	c.JSON(http.StatusOK, items)
}

// BotContactActivity returns one person's bot interactions (?phone=), most recent
// first — what that contact searched for / asked the bot.
func (h *ConsoleHandler) BotContactActivity(c *gin.Context) {
	phone := c.Query("phone")
	if phone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone is required"})
		return
	}
	items, err := h.st.ContactActivity(c.Request.Context(), phone, 200)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"activity": items, "count": len(items)})
}

