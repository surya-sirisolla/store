package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"store/backend/internal/models"
	"store/backend/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type BulkHandler struct {
	db       *gorm.DB
	excel    *services.ExcelService
	aiImport *services.AIImportService
}

func NewBulkHandler(db *gorm.DB, excel *services.ExcelService, aiImport *services.AIImportService) *BulkHandler {
	return &BulkHandler{db: db, excel: excel, aiImport: aiImport}
}

// InspectUpload parses the Excel and returns headers + Claude mapping suggestion.
func (h *BulkHandler) InspectUpload(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot open file"})
		return
	}
	defer f.Close()

	result, err := h.excel.Inspect(f)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// ConfirmImport runs the import after the user confirms the column mapping.
func (h *BulkHandler) ConfirmImport(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
		return
	}

	catID, _ := strconv.Atoi(c.PostForm("category_id"))
	if catID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "category_id required"})
		return
	}

	var mapping map[string]string
	if err := json.Unmarshal([]byte(c.PostForm("mapping")), &mapping); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mapping JSON"})
		return
	}

		var cat models.Category
	if err := h.db.First(&cat, catID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "category not found"})
		return
	}

	job := models.BulkUploadJob{FileName: fh.Filename, Status: "processing"}
	h.db.Create(&job)

	f, _ := fh.Open()
	defer f.Close()

	go h.excel.Import(f, uint(catID), mapping, job.ID)

	c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID, "message": "import started"})
}

// AIImportEstimate parses just enough of the sheet to report how many
// Claude calls AIImportExcel would make, so the console can show the
// expected API cost and get confirmation before actually running it.
func (h *BulkHandler) AIImportEstimate(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot open file"})
		return
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot read file"})
		return
	}

	rows, calls, err := h.aiImport.EstimateExcelRows(data)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"total_rows": rows, "estimated_calls": calls})
}

// AIImportExcel accepts an Excel file and extracts listings from it via
// Claude directly — no column mapping step, no target category to pick.
// Categories, sub-categories, quantity and price are all inferred from the
// sheet's contents. Runs as a background job, same as ConfirmImport.
func (h *BulkHandler) AIImportExcel(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot open file"})
		return
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot read file"})
		return
	}

	job := models.BulkUploadJob{FileName: fh.Filename, Status: "processing"}
	h.db.Create(&job)

	go h.aiImport.ImportExcel(data, job.ID)

	c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID, "message": "AI extraction started"})
}

func (h *BulkHandler) JobStatus(c *gin.Context) {
	var job models.BulkUploadJob
	if err := h.db.First(&job, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	c.JSON(http.StatusOK, job)
}

func (h *BulkHandler) ListJobs(c *gin.Context) {
	jobs := []models.BulkUploadJob{}
	h.db.Order("created_at DESC").Limit(20).Find(&jobs)
	c.JSON(http.StatusOK, jobs)
}
