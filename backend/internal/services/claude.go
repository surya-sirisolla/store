// Package services holds the Excel bulk-import helper and its Claude-assisted
// column mapping. (The WhatsApp agent itself is PicoClaw, not this backend.)
package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const claudeAPI = "https://api.anthropic.com/v1/messages"
const claudeModel = "claude-sonnet-4-6"

type ClaudeService struct {
	// keyFunc resolves the current API key at call time (it can change at runtime
	// when the owner sets it from the console).
	keyFunc func() string
}

func NewClaudeService(keyFunc func() string) *ClaudeService {
	return &ClaudeService{keyFunc: keyFunc}
}

// Enabled reports whether an API key is currently configured.
func (s *ClaudeService) Enabled() bool { return s.keyFunc() != "" }

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []claudeMessage `json:"messages"`
}

type claudeResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

func (s *ClaudeService) ask(prompt string) (string, error) {
	return s.askWithMaxTokens(prompt, 1024)
}

func (s *ClaudeService) askWithMaxTokens(prompt string, maxTokens int) (string, error) {
	body, _ := json.Marshal(claudeRequest{
		Model:     claudeModel,
		MaxTokens: maxTokens,
		Messages:  []claudeMessage{{Role: "user", Content: prompt}},
	})

	req, _ := http.NewRequest("POST", claudeAPI, bytes.NewReader(body))
	req.Header.Set("x-api-key", s.keyFunc())
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("claude API error %d: %s", resp.StatusCode, string(b))
	}

	var result claudeResponse
	json.Unmarshal(b, &result)
	if len(result.Content) == 0 {
		return "", fmt.Errorf("empty response from Claude")
	}
	return result.Content[0].Text, nil
}

// DetectExcelMapping asks Claude to map spreadsheet columns to the standard
// listing fields: name, phone, address, description, category, extra_*, ignore.
func (s *ClaudeService) DetectExcelMapping(headers []string, sampleRows [][]string) (string, error) {
	headerStr, _ := json.Marshal(headers)
	sampleStr, _ := json.Marshal(sampleRows)

	prompt := fmt.Sprintf(`You are a data mapping assistant.

Given these Excel column headers: %s

And these sample rows: %s

Map each column to one of these standard fields:
- name (business/person/product name)
- phone
- address
- description
- category
- extra_* (any other relevant field, prefix with extra_)
- ignore (skip this column)

Return ONLY a valid JSON object like:
{"column_header": "standard_field", ...}

No explanation, just the JSON.`, string(headerStr), string(sampleStr))

	return s.ask(prompt)
}

// ExtractListings asks Claude to turn arbitrary raw text (a pasted note, a
// catalog dump, a WhatsApp forward, anything) into a flat JSON array of
// listing items ready to insert.
func (s *ClaudeService) ExtractListings(rawText string) (string, error) {
	prompt := fmt.Sprintf(`You are a data extraction assistant.

Extract every product/item/service mentioned in the raw text below into a
JSON array. For each item produce an object with exactly these fields:
- "category": top-level category name (string, required — infer one if not explicit)
- "sub_category": a more specific sub-category name (string, or null if there isn't a sensible one)
- "name": the item/product/service name (string, required)
- "quantity": a number if a quantity/stock count is mentioned, otherwise null
- "price": a number if a price is mentioned (no currency symbols/commas), otherwise null
- "description": any other descriptive detail about the item (string, or null)

Raw text:
%s

Return ONLY a valid JSON array like:
[{"category": "...", "sub_category": "...", "name": "...", "quantity": null, "price": null, "description": "..."}]

No explanation, no markdown fences, just the JSON array.`, rawText)

	return s.askWithMaxTokens(prompt, 4096)
}
