package services

import (
	"encoding/json"
	"fmt"
	"mime/multipart"
	"strings"

	"store/backend/internal/models"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

type ExcelService struct {
	db     *gorm.DB
	claude *ClaudeService
}

func NewExcelService(db *gorm.DB, claude *ClaudeService) *ExcelService {
	return &ExcelService{db: db, claude: claude}
}

type MappingResult struct {
	Headers  []string          `json:"headers"`
	Mapping  map[string]string `json:"mapping"`
	Sample   [][]string        `json:"sample"`
	RowCount int               `json:"row_count"`
}

// Inspect opens an uploaded Excel file, extracts headers and sample rows,
// then calls Claude to suggest column mapping. Returns the suggestion for user confirmation.
func (s *ExcelService) Inspect(file multipart.File) (*MappingResult, error) {
	f, err := excelize.OpenReader(file)
	if err != nil {
		return nil, fmt.Errorf("cannot read excel: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sheets found")
	}

	rows, err := f.GetRows(sheets[0])
	if err != nil || len(rows) < 2 {
		return nil, fmt.Errorf("sheet is empty or has only headers")
	}

	headers := rows[0]
	sample := rows[1:]
	if len(sample) > 5 {
		sample = sample[:5]
	}

	mapping := map[string]string{}
	if s.claude.Enabled() {
		raw, err := s.claude.DetectExcelMapping(headers, sample)
		if err == nil {
			json.Unmarshal([]byte(raw), &mapping)
		}
	}

	return &MappingResult{
		Headers:  headers,
		Mapping:  mapping,
		Sample:   sample,
		RowCount: len(rows) - 1,
	}, nil
}

// Import bulk-inserts rows using a confirmed column mapping.
func (s *ExcelService) Import(
	file multipart.File,
	categoryID uint,
	mapping map[string]string, // header -> standard field
	jobID uint,
) error {
	f, err := excelize.OpenReader(file)
	if err != nil {
		return err
	}
	defer f.Close()

	sheets := f.GetSheetList()
	rows, _ := f.GetRows(sheets[0])
	if len(rows) < 2 {
		return fmt.Errorf("no data rows")
	}

	headers := rows[0]
	inserted, errCount := 0, 0

	for _, row := range rows[1:] {
		listing := models.Listing{
			CategoryID: categoryID,
			Active:     true,
			Data:       models.JSONB{},
		}

		for i, cell := range row {
			if i >= len(headers) {
				break
			}
			field := mapping[headers[i]]
			switch field {
			case "name":
				listing.Name = cell
			case "phone":
				listing.Phone = cell
			case "address":
				listing.Address = cell
			case "description":
				listing.Description = cell
			default:
				if strings.HasPrefix(field, "extra_") {
					listing.Data[field] = cell
				}
			}
		}

		if listing.Name == "" {
			errCount++
			continue
		}

		if err := s.db.Create(&listing).Error; err != nil {
			errCount++
		} else {
			inserted++
		}
	}

	s.db.Model(&models.BulkUploadJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
		"status":    "done",
		"inserted":  inserted,
		"errors":    errCount,
		"total_rows": len(rows) - 1,
	})

	return nil
}
