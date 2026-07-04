package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"store/backend/internal/models"

	"github.com/gin-gonic/gin"
)

// GetProfile returns the single business profile (or an empty object if unset).
func (h *ConsoleHandler) GetProfile(c *gin.Context) {
	p, err := h.st.GetBusinessProfile(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if p == nil {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	c.JSON(http.StatusOK, p)
}

// UpdateProfile upserts the single business profile (owner-only).
func (h *ConsoleHandler) UpdateProfile(c *gin.Context) {
	var input models.BusinessProfile
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if input.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if err := h.st.UpsertBusinessProfile(c.Request.Context(), &input); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, input)
}

// ListLocations returns every business location (godown), each with its
// owner-editable address. Sourced from the Livekeeping sync.
func (h *ConsoleHandler) ListLocations(c *gin.Context) {
	locs, err := h.st.ListBusinessLocations(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, locs)
}

// UpdateLocation saves the owner's edits to one location (address/name/active).
// SourceID is left untouched so the next sync still matches this row.
func (h *ConsoleHandler) UpdateLocation(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var input struct {
		Name    string `json:"name"`
		Address string `json:"address"`
		Area    string `json:"area"`
		City    string `json:"city"`
		State   string `json:"state"`
		Pincode string `json:"pincode"`
		Phone   string `json:"phone"`
		Active  *bool  `json:"active"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	loc, err := h.st.GetBusinessLocation(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if loc == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "location not found"})
		return
	}

	// Name defaults to the current one so an address-only edit can't blank it.
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = loc.Name
	}
	active := loc.Active
	if input.Active != nil {
		active = *input.Active
	}

	edit := models.BusinessLocation{
		Name:    name,
		Address: strings.TrimSpace(input.Address),
		Area:    strings.TrimSpace(input.Area),
		City:    strings.TrimSpace(input.City),
		State:   strings.TrimSpace(input.State),
		Pincode: strings.TrimSpace(input.Pincode),
		Phone:   strings.TrimSpace(input.Phone),
		Active:  active,
	}
	if err := h.st.UpdateBusinessLocation(c.Request.Context(), uint(id), edit); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	updated, _ := h.st.GetBusinessLocation(c.Request.Context(), uint(id))
	c.JSON(http.StatusOK, updated)
}
