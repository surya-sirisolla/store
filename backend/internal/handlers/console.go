package handlers

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"store/backend/internal/models"
	"store/backend/internal/store"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ConsoleHandler serves the Owner console: categories, listings, staff, business
// profile and bot monitoring. One deployment serves one business, so there is
// nothing to scope — every route operates on the single catalog.
type ConsoleHandler struct {
	db *gorm.DB
	st *store.Store
}

func NewConsoleHandler(db *gorm.DB, st *store.Store) *ConsoleHandler {
	return &ConsoleHandler{db: db, st: st}
}

func toSlug(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), " ", "-")
}

// ── Categories ────────────────────────────────────────────────────────────────

func (h *ConsoleHandler) ListCategories(c *gin.Context) {
	cats, err := h.st.ListCategoryTree(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cats)
}

func (h *ConsoleHandler) CreateCategory(c *gin.Context) {
	var input struct {
		Name     string `json:"name" binding:"required"`
		ParentID *uint  `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	level := 0
	if input.ParentID != nil {
		var parent models.Category
		if err := h.db.First(&parent, *input.ParentID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "parent category not found"})
			return
		}
		level = parent.Level + 1
	}

	cat := models.Category{
		Name:     input.Name,
		Slug:     toSlug(input.Name),
		ParentID: input.ParentID,
		Level:    level,
	}
	h.db.Create(&cat)
	c.JSON(http.StatusCreated, cat)
}

func (h *ConsoleHandler) DeleteCategory(c *gin.Context) {
	if h.db.Delete(&models.Category{}, c.Param("id")).RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "category not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ── Listings ──────────────────────────────────────────────────────────────────

func (h *ConsoleHandler) ListListings(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	const limit = 15

	stock := c.Query("stock")

	// Free-text search uses the shared tokenized search; otherwise a plain page.
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		items, err := h.st.SearchListings(c.Request.Context(), store.SearchParams{
			Query: q, Category: c.Query("category"), Limit: limit, Stock: stock,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": items, "total": len(items), "page": 1})
		return
	}

	query := h.db.Model(&models.Listing{})
	if cat := c.Query("category_id"); cat != "" {
		query = query.Where("category_id = ?", cat)
	}
	if cond := store.StockCondition(stock); cond != "" {
		query = query.Where(cond)
	}

	var total int64
	query.Count(&total)

	items := []models.Listing{}
	query.Preload("Category").
		Order("created_at DESC").
		Limit(limit).Offset((page - 1) * limit).
		Find(&items)

	c.JSON(http.StatusOK, gin.H{"data": items, "total": total, "page": page})
}

func (h *ConsoleHandler) CreateListing(c *gin.Context) {
	var input struct {
		Name        string       `json:"name" binding:"required"`
		Phone       string       `json:"phone"`
		Address     string       `json:"address"`
		Description string       `json:"description"`
		CategoryID  uint         `json:"category_id" binding:"required"`
		Quantity    *int         `json:"quantity"`
		Price       *float64     `json:"price"`
		Data        models.JSONB `json:"data"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var cat models.Category
	if err := h.db.First(&cat, input.CategoryID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "category not found"})
		return
	}

	listing := models.Listing{
		CategoryID:  input.CategoryID,
		Name:        input.Name,
		Phone:       input.Phone,
		Address:     input.Address,
		Description: input.Description,
		Quantity:    input.Quantity,
		Price:       input.Price,
		Data:        input.Data,
		Active:      true,
	}
	h.db.Create(&listing)
	// A newly-added item may be something a customer is waitlisted for.
	if listing.Quantity == nil || *listing.Quantity > 0 {
		h.st.RaiseReadyAlerts(c.Request.Context())
	}
	c.JSON(http.StatusCreated, listing)
}

func (h *ConsoleHandler) UpdateListing(c *gin.Context) {
	var listing models.Listing
	if err := h.db.First(&listing, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "listing not found"})
		return
	}

	var input struct {
		Name        *string      `json:"name"`
		Phone       *string      `json:"phone"`
		Address     *string      `json:"address"`
		Description *string      `json:"description"`
		CategoryID  *uint        `json:"category_id"`
		Quantity    *int         `json:"quantity"`
		Price       *float64     `json:"price"`
		Featured    *bool        `json:"featured"`
		OfferText   *string      `json:"offer_text"`
		Data        models.JSONB `json:"data"`
		Active      *bool        `json:"active"`
	}
	c.ShouldBindJSON(&input)

	if input.Name != nil {
		listing.Name = *input.Name
	}
	if input.Phone != nil {
		listing.Phone = *input.Phone
	}
	if input.Address != nil {
		listing.Address = *input.Address
	}
	if input.Description != nil {
		listing.Description = *input.Description
	}
	if input.CategoryID != nil {
		listing.CategoryID = *input.CategoryID
	}
	if input.Quantity != nil {
		listing.Quantity = input.Quantity
	}
	if input.Price != nil {
		listing.Price = input.Price
	}
	if input.Featured != nil {
		listing.Featured = *input.Featured
	}
	if input.OfferText != nil {
		listing.OfferText = *input.OfferText
	}
	if input.Data != nil {
		listing.Data = input.Data
	}
	if input.Active != nil {
		listing.Active = *input.Active
	}

	h.db.Save(&listing)
	// A restock (quantity back above zero) may satisfy a waiting customer.
	if listing.Active && (listing.Quantity == nil || *listing.Quantity > 0) {
		h.st.RaiseReadyAlerts(c.Request.Context())
	}
	c.JSON(http.StatusOK, listing)
}

func (h *ConsoleHandler) DeleteListing(c *gin.Context) {
	if h.db.Delete(&models.Listing{}, c.Param("id")).RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "listing not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ── Staff ────────────────────────────────────────────────────────────────────
// Staff records have no login — they exist only to register a WhatsApp number
// the bot recognizes for staff-only tools (e.g. who messaged today, pending
// alerts) and to let the business reach them.

func (h *ConsoleHandler) ListStaff(c *gin.Context) {
	users := []models.User{}
	h.db.Order("name").Find(&users)
	c.JSON(http.StatusOK, users)
}

func (h *ConsoleHandler) CreateStaff(c *gin.Context) {
	var input struct {
		Name  string `json:"name" binding:"required"`
		Email string `json:"email" binding:"required,email"`
		Phone string `json:"phone" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	phone, ok := normalizePhone(input.Phone)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "phone must include country code, e.g. +919876543210"})
		return
	}

	user := models.User{
		Name: input.Name, Email: input.Email,
		Phone: phone, Active: true,
	}
	if err := h.db.Create(&user).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "email already in use"})
		return
	}
	c.JSON(http.StatusCreated, user)
}

// e164 matches an international phone number: a leading +, a non-zero country
// code digit, then 7–14 more digits.
var e164 = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)

// normalizePhone strips spaces, dashes and parentheses, then validates the
// result as E.164. Returns the normalized number and whether it's valid.
func normalizePhone(raw string) (string, bool) {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '(', ')':
			return -1
		}
		return r
	}, strings.TrimSpace(raw))
	if !e164.MatchString(cleaned) {
		return "", false
	}
	return cleaned, true
}

func (h *ConsoleHandler) DeleteStaff(c *gin.Context) {
	res := h.db.Delete(&models.User{}, c.Param("id"))
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "staff not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
