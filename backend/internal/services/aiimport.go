// Package services: AIImportService turns arbitrary raw text — pasted notes
// or whole Excel sheets — into listings using Claude, resolving/creating
// categories and sub-categories as it goes.
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"store/backend/internal/models"
	"store/backend/internal/store"

	"github.com/xuri/excelize/v2"
)

type AIImportService struct {
	st     *store.Store
	claude *ClaudeService
}

func NewAIImportService(st *store.Store, claude *ClaudeService) *AIImportService {
	return &AIImportService{st: st, claude: claude}
}

// extractedItem mirrors the JSON shape Claude is asked to return.
type extractedItem struct {
	Category    string   `json:"category"`
	SubCategory string   `json:"sub_category"`
	Name        string   `json:"name"`
	Quantity    *float64 `json:"quantity"`
	Price       *float64 `json:"price"`
	Description string   `json:"description"`
}

// ImportResult summarizes what happened with an AI-driven import.
type ImportResult struct {
	Inserted int      `json:"inserted"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
}

// Import extracts listings from rawText via Claude and inserts them,
// creating any missing categories/sub-categories along the way.
func (s *AIImportService) Import(ctx context.Context, rawText string) (*ImportResult, error) {
	if !s.claude.Enabled() {
		return nil, fmt.Errorf("AI key not configured")
	}
	rawText = strings.TrimSpace(rawText)
	if rawText == "" {
		return nil, fmt.Errorf("raw_text required")
	}

	items, err := s.extractItems(rawText)
	if err != nil {
		return nil, err
	}

	result := &ImportResult{}
	s.insertItems(ctx, s.st, items, result)
	return result, nil
}

// excelBatchSize caps how many spreadsheet rows go into a single Claude call
// so the prompt/response stay within token limits on large sheets.
const excelBatchSize = 25

// ImportExcel extracts listings from an uploaded Excel file via Claude — no
// column mapping needed, it reads the sheet the same way it'd read raw text.
// Runs in a background goroutine kicked off by the handler, so it reports
// progress through the BulkUploadJob row rather than a return value.
func (s *AIImportService) ImportExcel(fileBytes []byte, jobID uint) {
	result, totalRows, err := s.importExcelBytes(context.Background(), fileBytes)

	updates := map[string]interface{}{"total_rows": totalRows}
	if err != nil {
		updates["status"] = "failed"
		updates["error_detail"] = err.Error()
	} else {
		updates["status"] = "done"
		updates["inserted"] = result.Inserted
		updates["errors"] = len(result.Errors)
		if len(result.Errors) > 0 {
			updates["error_detail"] = strings.Join(result.Errors, "; ")
		}
	}
	s.st.DB().Model(&models.BulkUploadJob{}).Where("id = ?", jobID).Updates(updates)
}

func (s *AIImportService) importExcelBytes(ctx context.Context, fileBytes []byte) (*ImportResult, int, error) {
	if !s.claude.Enabled() {
		return nil, 0, fmt.Errorf("AI key not configured")
	}

	f, err := excelize.OpenReader(bytes.NewReader(fileBytes))
	if err != nil {
		return nil, 0, fmt.Errorf("cannot read excel: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, 0, fmt.Errorf("no sheets found")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil || len(rows) < 2 {
		return nil, 0, fmt.Errorf("sheet is empty or has only headers")
	}

	headers := rows[0]
	dataRows := rows[1:]
	result := &ImportResult{}
	st := s.st

	for start := 0; start < len(dataRows); start += excelBatchSize {
		end := start + excelBatchSize
		if end > len(dataRows) {
			end = len(dataRows)
		}

		items, err := s.extractItems(rowsToText(headers, dataRows[start:end]))
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("rows %d-%d: %v", start+2, end+1, err))
			continue
		}
		s.insertItems(ctx, st, items, result)
	}

	return result, len(dataRows), nil
}

// EstimateExcelRows reports how many data rows a sheet has and how many
// Claude calls ImportExcel would make at the current batch size, so the
// caller can show the cost and get confirmation before burning API quota.
func (s *AIImportService) EstimateExcelRows(fileBytes []byte) (dataRows int, calls int, err error) {
	f, err := excelize.OpenReader(bytes.NewReader(fileBytes))
	if err != nil {
		return 0, 0, fmt.Errorf("cannot read excel: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return 0, 0, fmt.Errorf("no sheets found")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil || len(rows) < 2 {
		return 0, 0, fmt.Errorf("sheet is empty or has only headers")
	}

	dataRows = len(rows) - 1
	calls = (dataRows + excelBatchSize - 1) / excelBatchSize
	return dataRows, calls, nil
}

// rowsToText renders a header row + data rows as simple delimited text so
// Claude can read it like any other raw text, regardless of column layout.
func rowsToText(headers []string, rows [][]string) string {
	var b strings.Builder
	b.WriteString(strings.Join(headers, " | "))
	b.WriteByte('\n')
	for _, row := range rows {
		b.WriteString(strings.Join(row, " | "))
		b.WriteByte('\n')
	}
	return b.String()
}

// extractItems calls Claude on a block of text and parses its JSON-array response.
func (s *AIImportService) extractItems(text string) ([]extractedItem, error) {
	raw, err := s.claude.ExtractListings(text)
	if err != nil {
		return nil, fmt.Errorf("claude extraction failed: %w", err)
	}
	items, err := parseExtractedItems(raw)
	if err != nil {
		return nil, fmt.Errorf("could not parse AI response: %w", err)
	}
	return items, nil
}

// insertItems resolves categories/sub-categories and inserts each extracted
// item, tallying outcomes into result.
func (s *AIImportService) insertItems(ctx context.Context, st *store.Store, items []extractedItem, result *ImportResult) {
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			result.Skipped++
			continue
		}

		catID, err := st.FindOrCreateCategory(ctx, item.Category, item.SubCategory)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", name, err))
			continue
		}

		listing := models.Listing{
			CategoryID:  catID,
			Name:        name,
			Description: item.Description,
			Price:       item.Price,
			Active:      true,
			Data:        models.JSONB{},
		}
		if item.Quantity != nil {
			q := int(*item.Quantity)
			listing.Quantity = &q
		}

		if err := st.DB().WithContext(ctx).Create(&listing).Error; err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		result.Inserted++
	}
}

// parseExtractedItems unmarshals Claude's JSON array, tolerating a leading
// ```json fence in case the model adds one despite instructions not to.
func parseExtractedItems(raw string) ([]extractedItem, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var items []extractedItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, err
	}
	return items, nil
}
