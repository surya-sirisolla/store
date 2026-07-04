package handlers

import (
	"net/http"

	"store/backend/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// JobsHandler exposes the console's Automation section: listing scheduled jobs,
// saving their schedules, and triggering a run on demand.
type JobsHandler struct {
	svc *services.JobService
}

func NewJobsHandler(svc *services.JobService) *JobsHandler {
	return &JobsHandler{svc: svc}
}

// List returns every scheduled job with its schedule + last outcome.
func (h *JobsHandler) List(c *gin.Context) {
	jobs, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, jobs)
}

// SaveSchedule persists a job's schedule (manual | interval | daily).
func (h *JobsHandler) SaveSchedule(c *gin.Context) {
	var in services.ScheduleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	job, err := h.svc.SaveSchedule(c.Param("key"), in)
	if err == gorm.ErrRecordNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such job"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, job)
}

// Preview builds the message body + recipient count for a prospective broadcast,
// without sending — powers the create/edit form.
func (h *JobsHandler) Preview(c *gin.Context) {
	var in services.BroadcastInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	body, count, err := h.svc.PreviewBroadcast(c.Request.Context(), in.Config)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"preview": body, "recipients": count})
}

// Create makes a new owner-defined reminder/broadcast job.
func (h *JobsHandler) Create(c *gin.Context) {
	var in services.BroadcastInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	job, err := h.svc.CreateBroadcast(in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, job)
}

// Update edits an existing broadcast job.
func (h *JobsHandler) Update(c *gin.Context) {
	var in services.BroadcastInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	job, err := h.svc.UpdateBroadcast(c.Param("key"), in)
	if err == gorm.ErrRecordNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such job"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, job)
}

// Delete removes an owner-created job (system jobs are protected).
func (h *JobsHandler) Delete(c *gin.Context) {
	err := h.svc.Delete(c.Param("key"))
	if err == gorm.ErrRecordNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such job"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// Run triggers a job immediately. It returns as soon as the job is kicked off —
// the run itself happens in the background; poll List for progress/outcome.
func (h *JobsHandler) Run(c *gin.Context) {
	err := h.svc.RunNow(c.Param("key"))
	if err == gorm.ErrRecordNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such job"})
		return
	}
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "started"})
}
