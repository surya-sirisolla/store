// Package services — livekeeping.go pulls the business's real stock items from
// the Livekeeping cloud inventory (goapi.livekeeping.com) and upserts them into
// our Postgres directory so the console and the WhatsApp bot answer from the
// authoritative catalog instead of hand-entered data.
package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"store/backend/internal/models"
	"store/backend/internal/store"

	"gorm.io/gorm"
)

const (
	livekeepingStockItemsURL = "https://goapi.livekeeping.com/v1/stockItems/getStockItems"
	livekeepingCompanyURL    = "https://goapi.livekeeping.com/v1/companies/getCompany"
	livekeepingGodownsURL    = "https://goapi.livekeeping.com/v2/godown/getGodownsSummary"
)

// livekeepingPageSize is how many items we pull per request. The API paginates
// via startLimit (offset) + endLimit (count) and HARD-CAPS endLimit at 100 —
// asking for more makes it reject the call ("End Limit cannot be greater than
// 100"). So 100 is both the max and our page size (~54 round trips for ~5k items).
const livekeepingPageSize = 100

// livekeepingMaxPages caps the loop so a misbehaving totalCount can't spin
// forever. 500 pages * 500 = 250k items, far beyond any realistic catalog.
const livekeepingMaxPages = 500

// Rate-limit / resilience tuning. We stay deliberately gentle: strictly
// sequential requests, a short pause between pages so we're never bursty, and
// bounded exponential backoff on 429/5xx so a transient throttle doesn't fail
// the whole sync.
const (
	livekeepingPageDelay   = 400 * time.Millisecond // polite gap between pages
	livekeepingMaxAttempts = 4                      // per-page tries before giving up
	livekeepingBaseBackoff = 1 * time.Second        // 1s → 2s → 4s …
	livekeepingMaxBackoff  = 30 * time.Second       // ceiling for any single wait
)

// LivekeepingService performs the stock-item sync.
type LivekeepingService struct {
	db     *gorm.DB
	st     *store.Store
	client *http.Client
}

func NewLivekeepingService(db *gorm.DB, st *store.Store) *LivekeepingService {
	return &LivekeepingService{
		db:     db,
		st:     st,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// SyncCreds are the credentials + account ids for one sync run.
type SyncCreds struct {
	Token     string
	CompanyID string
	UserID    string
}

// SyncResult summarizes what a sync did.
type SyncResult struct {
	Total   int `json:"total"`   // items reported by the API (totalCount)
	Fetched int `json:"fetched"` // items we actually processed
	Created int `json:"created"` // new listings inserted
	Updated int `json:"updated"` // existing listings whose fields changed
	Deleted int `json:"deleted"` // previously-synced listings gone from Livekeeping
	Errors  int `json:"errors"`

	CompanyUpdated bool `json:"company_updated"` // business profile refreshed from Livekeeping

	// Godown/location reconciliation. Each Livekeeping godown maps to one
	// BusinessLocation row; owner-entered addresses are preserved across syncs.
	LocationsCreated int `json:"locations_created"`
	LocationsUpdated int `json:"locations_updated"`
	LocationsDeleted int `json:"locations_deleted"`
}

// companyResponse mirrors the getCompany envelope. The business-profile fields we
// care about live under data.companyDetails; the name is data.basicCompanyName
// (cleaner) or data.companyName.
type companyResponse struct {
	Data struct {
		ID               string `json:"id"`
		CompanyName      string `json:"companyName"`
		BasicCompanyName string `json:"basicCompanyName"`
		CompanyDetails   struct {
			Address      string `json:"address"`
			GstNumber    string `json:"gstNumber"`
			StateName    string `json:"stateName"`
			Pincode      string `json:"pincode"`
			EMail        string `json:"eMail"`
			MobileNumber string `json:"mobileNumber"`
			PhoneNumber  string `json:"phoneNumber"`
			Country      string `json:"country"`
			Website      string `json:"website"`
		} `json:"companyDetails"`
	} `json:"data"`
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// AuthError marks a failure caused by a rejected/expired token (HTTP 401/403).
// Callers use it to flip the cached token-validity flag and prompt for a fresh
// token, rather than treating it as a transient/server error.
type AuthError struct{ msg string }

func (e *AuthError) Error() string { return e.msg }

// stockItem is the subset of a Livekeeping stock item we care about. All the
// numeric fields arrive as strings (and may be negative or fractional).
type stockItem struct {
	ID             string `json:"id"`
	StockItemName  string `json:"stockItemName"`
	Guid           string `json:"guid"`
	HsnCode        string `json:"hsnCode"`
	OpeningBalance string `json:"openingBalance"`
	ClosingBalance string `json:"closingBalance"`
	ClosingRate    string `json:"closingRate"`
	ClosingValue   string `json:"closingValue"`
	AvgRate        string `json:"avgRate"`
	TotalAmount    string `json:"totalAmount"`
	Parent         string `json:"parent"`
	Category       string `json:"category"`
	BaseUnits      string `json:"baseUnits"`
	TotalQuantity  string `json:"totalQuantity"`
}

// stockResponse mirrors the API envelope: the item page lives under "data". The
// API sometimes signals failure in-body (HTTP 200 with a non-2xx "status" and a
// "message") instead of via the HTTP status, so we capture those too.
type stockResponse struct {
	Data struct {
		List       []stockItem `json:"list"`
		TotalCount int         `json:"totalCount"`
	} `json:"data"`
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// godown is one location from getGodownsSummary. godownGuid is the stable id we
// key on; godownName is the label the owner sees.
type godown struct {
	GodownName string `json:"godownName"`
	GodownGuid string `json:"godownGuid"`
}

// godownResponse mirrors the getGodownsSummary envelope (data.list + totalCount).
type godownResponse struct {
	Data struct {
		List       []godown `json:"list"`
		TotalCount int      `json:"totalCount"`
	} `json:"data"`
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// CheckToken makes the cheapest possible call (one item) to see whether the
// saved token is still valid. Returns nil if it works, an *AuthError if the
// token is rejected/expired, or another error for network/parse trouble.
func (s *LivekeepingService) CheckToken(ctx context.Context, creds SyncCreds) error {
	if strings.TrimSpace(creds.Token) == "" {
		return fmt.Errorf("livekeeping token is required")
	}
	creds.UserID = effectiveUserID(creds)
	_, err := s.attemptFetch(ctx, creds, 0, 1)
	if re, ok := err.(*retryableError); ok {
		return re.err // surface the underlying transient reason without retrying
	}
	return err
}

// SyncScope selects which parts of Livekeeping a Sync run imports. The owner
// configures this per integration; an all-false scope is a no-op.
type SyncScope struct {
	Stock   bool // stock items → listings
	Godowns bool // godowns → business locations
	Profile bool // company profile → business profile
}

// FullScope imports everything — the default when no selection is given.
func FullScope() SyncScope { return SyncScope{Stock: true, Godowns: true, Profile: true} }

// Sync reconciles our directory with the Livekeeping catalog incrementally: it
// matches items by their stable Livekeeping id (SourceID), updating the ones
// that changed, inserting new ones, and deleting only previously-synced items
// that have vanished from Livekeeping. Manually-added listings (no SourceID)
// are left untouched. To avoid ever mangling data on a bad token or a mid-run
// failure, it fetches the whole catalog into memory first and only touches the
// DB once every page is in hand. The scope selects which parts run.
func (s *LivekeepingService) Sync(ctx context.Context, creds SyncCreds, scope SyncScope) (SyncResult, error) {
	var res SyncResult
	if strings.TrimSpace(creds.Token) == "" {
		return res, fmt.Errorf("livekeeping token is required")
	}
	// The user id lives in the token; use it so the owner only manages the token
	// (+ company id). Falls back to any stored id if the token can't be decoded.
	creds.UserID = effectiveUserID(creds)

	// 0. Company profile — one polite call per sync to keep the business profile
	// fresh. A bad token surfaces here (and would also fail the stock pull); any
	// other company error is non-fatal so it never blocks the stock sync.
	if scope.Profile {
		if err := s.syncCompanyProfile(ctx, creds); err != nil {
			var authErr *AuthError
			if errors.As(err, &authErr) {
				return res, err
			}
		} else {
			res.CompanyUpdated = true
		}
		if err := sleepCtx(ctx, livekeepingPageDelay); err != nil {
			return res, err
		}
	}

	// 0b. Godowns → business locations. Same policy as the company profile: a
	// bad token aborts the sync, any other godown error is non-fatal so it never
	// blocks the stock pull.
	if scope.Godowns {
		if err := s.syncGodowns(ctx, creds, &res); err != nil {
			var authErr *AuthError
			if errors.As(err, &authErr) {
				return res, err
			}
		}
		if err := sleepCtx(ctx, livekeepingPageDelay); err != nil {
			return res, err
		}
	}

	if !scope.Stock {
		return res, nil // stock (and its prune) skipped by scope
	}

	// 1. Pull the whole catalog (network only — no DB writes yet). Pages are
	// fetched strictly one at a time with a short pause between them so we never
	// burst against the API's rate limiter.
	var items []stockItem
	start := 0
	for page := 0; page < livekeepingMaxPages; page++ {
		if page > 0 {
			if err := sleepCtx(ctx, livekeepingPageDelay); err != nil {
				return res, err
			}
		}
		resp, err := s.fetchPage(ctx, creds, start, livekeepingPageSize)
		if err != nil {
			return res, err
		}
		if page == 0 {
			res.Total = resp.Data.TotalCount
		}
		if len(resp.Data.List) == 0 {
			break
		}
		items = append(items, resp.Data.List...)
		start += len(resp.Data.List)
		if res.Total > 0 && start >= res.Total {
			break
		}
	}

	// 2. Load the listings we synced before, keyed by SourceID, so we can tell
	// new items from changed ones and detect deletions. Manual listings (empty
	// source_id) are excluded here and thus never pruned.
	existing := map[string]models.Listing{}
	var prev []models.Listing
	if err := s.db.WithContext(ctx).Where("source_id <> ''").Find(&prev).Error; err != nil {
		return res, fmt.Errorf("loading existing listings: %w", err)
	}
	for _, l := range prev {
		existing[l.SourceID] = l
	}

	// 3. Upsert each item, reusing category rows (FindOrCreateCategory is
	// idempotent) and only writing rows whose fields actually changed.
	seen := make(map[string]bool, len(items))
	for _, it := range items {
		sourceID := itemSourceID(it)
		if sourceID != "" {
			seen[sourceID] = true
		}
		outcome, err := s.upsertItem(ctx, it, existing)
		if err != nil {
			res.Errors++
			continue
		}
		res.Fetched++
		switch outcome {
		case outcomeCreated:
			res.Created++
		case outcomeUpdated:
			res.Updated++
		}
	}

	// 4. Prune: previously-synced listings no longer present in Livekeeping.
	var stale []uint
	for id, l := range existing {
		if !seen[id] {
			stale = append(stale, l.ID)
		}
	}
	if len(stale) > 0 {
		if err := s.db.WithContext(ctx).Where("id IN ?", stale).Delete(&models.Listing{}).Error; err != nil {
			return res, fmt.Errorf("pruning removed listings: %w", err)
		}
		res.Deleted = len(stale)
	}
	return res, nil
}

// retryableError marks a failure worth retrying (rate limit, transient 5xx, or
// a network blip). It optionally carries the server's requested wait.
type retryableError struct {
	retryAfter time.Duration
	err        error
}

func (e *retryableError) Error() string { return e.err.Error() }

// fetchPage fetches one page, retrying on rate limits / transient errors with
// bounded exponential backoff (honoring Retry-After when the server sends it).
// Auth failures and other 4xx are returned immediately — retrying won't help.
func (s *LivekeepingService) fetchPage(ctx context.Context, creds SyncCreds, start, count int) (stockResponse, error) {
	var lastErr error
	for attempt := 0; attempt < livekeepingMaxAttempts; attempt++ {
		out, err := s.attemptFetch(ctx, creds, start, count)
		if err == nil {
			return out, nil
		}
		re, ok := err.(*retryableError)
		if !ok {
			return out, err // non-retryable (bad token, parse error, etc.)
		}
		lastErr = re.err
		if attempt == livekeepingMaxAttempts-1 {
			break
		}
		// Wait the larger of exponential backoff and the server's Retry-After.
		wait := livekeepingBaseBackoff << attempt
		if wait > livekeepingMaxBackoff {
			wait = livekeepingMaxBackoff
		}
		if re.retryAfter > wait {
			wait = re.retryAfter
		}
		if err := sleepCtx(ctx, wait); err != nil {
			return out, err
		}
	}
	return stockResponse{}, fmt.Errorf("livekeeping still failing after %d attempts: %w", livekeepingMaxAttempts, lastErr)
}

// attemptFetch performs a single request to the Livekeeping API. Retryable
// conditions (429, 5xx, network errors) are wrapped in *retryableError.
func (s *LivekeepingService) attemptFetch(ctx context.Context, creds SyncCreds, start, count int) (stockResponse, error) {
	var out stockResponse

	payload := map[string]interface{}{
		"company_id":  creds.CompanyID,
		"startLimit":  start,
		"endLimit":    count,
		"filterBy":    "Show All",
		"orderBy":     1,
		"requestFrom": "WEB",
		"searchTerm":  "",
		"sortBy":      "stockItemName",
		"_userId":     creds.UserID,
	}
	raw, err := s.sendLivekeeping(ctx, livekeepingStockItemsURL, payload, creds.Token)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("could not parse livekeeping response: %w", err)
	}
	// The API can report failure in-body with a non-2xx status while the HTTP
	// status is 200 (e.g. "End Limit cannot be greater than 100"). Surface it.
	if out.Status != 0 && (out.Status < 200 || out.Status >= 300) {
		msg := strings.TrimSpace(out.Message)
		if msg == "" {
			msg = fmt.Sprintf("status %d", out.Status)
		}
		return out, fmt.Errorf("livekeeping rejected the request: %s", msg)
	}
	return out, nil
}

// sendLivekeeping posts a JSON payload to a Livekeeping endpoint and returns the
// raw response body. Rate limits / 5xx / network blips are wrapped as
// *retryableError; auth failures as *AuthError. Shared by the stock-item and
// company calls so both classify HTTP status the same way.
func (s *LivekeepingService) sendLivekeeping(ctx context.Context, url string, payload map[string]interface{}, token string) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("token", token)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, &retryableError{err: fmt.Errorf("livekeeping request failed: %w", err)}
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, &retryableError{
			retryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
			err:        fmt.Errorf("livekeeping rate-limited the request (HTTP 429)"),
		}
	case resp.StatusCode >= 500:
		return nil, &retryableError{
			retryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
			err:        fmt.Errorf("livekeeping server error (HTTP %d)", resp.StatusCode),
		}
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, &AuthError{msg: fmt.Sprintf("livekeeping rejected the token (HTTP %d) — paste a fresh token", resp.StatusCode)}
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, fmt.Errorf("livekeeping returned HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// fetchCompany fetches the company profile with the same bounded retry/backoff as
// the stock pages (honoring Retry-After); auth/other 4xx return immediately.
func (s *LivekeepingService) fetchCompany(ctx context.Context, creds SyncCreds) (companyResponse, error) {
	var lastErr error
	for attempt := 0; attempt < livekeepingMaxAttempts; attempt++ {
		out, err := s.attemptCompany(ctx, creds)
		if err == nil {
			return out, nil
		}
		re, ok := err.(*retryableError)
		if !ok {
			return out, err
		}
		lastErr = re.err
		if attempt == livekeepingMaxAttempts-1 {
			break
		}
		wait := livekeepingBaseBackoff << attempt
		if wait > livekeepingMaxBackoff {
			wait = livekeepingMaxBackoff
		}
		if re.retryAfter > wait {
			wait = re.retryAfter
		}
		if err := sleepCtx(ctx, wait); err != nil {
			return out, err
		}
	}
	return companyResponse{}, fmt.Errorf("livekeeping company still failing after %d attempts: %w", livekeepingMaxAttempts, lastErr)
}

// attemptCompany performs a single getCompany request.
func (s *LivekeepingService) attemptCompany(ctx context.Context, creds SyncCreds) (companyResponse, error) {
	var out companyResponse
	payload := map[string]interface{}{
		"_userId":     creds.UserID,
		"company_id":  creds.CompanyID,
		"requestFrom": "WEB",
	}
	raw, err := s.sendLivekeeping(ctx, livekeepingCompanyURL, payload, creds.Token)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("could not parse livekeeping company response: %w", err)
	}
	if out.Status != 0 && (out.Status < 200 || out.Status >= 300) {
		msg := strings.TrimSpace(out.Message)
		if msg == "" {
			msg = fmt.Sprintf("status %d", out.Status)
		}
		return out, fmt.Errorf("livekeeping rejected the request: %s", msg)
	}
	return out, nil
}

// syncCompanyProfile pulls the company profile and folds it into our single
// BusinessProfile row. The profile is deliberately minimal now — identity and
// contact only — so we refresh the name, email and phone numbers Livekeeping
// provides and preserve the owner-set hours. Per-branch street addresses are
// handled by syncGodowns → BusinessLocation, not here.
func (s *LivekeepingService) syncCompanyProfile(ctx context.Context, creds SyncCreds) error {
	resp, err := s.fetchCompany(ctx, creds)
	if err != nil {
		return err
	}
	cd := resp.Data.CompanyDetails

	existing, _ := s.st.GetBusinessProfile(ctx)
	p := &models.BusinessProfile{}
	if existing != nil {
		*p = *existing
	}

	// Sync the name from Livekeeping on every run too (basicCompanyName is the
	// cleaner form; fall back to companyName).
	name := strings.TrimSpace(resp.Data.BasicCompanyName)
	if name == "" {
		name = strings.TrimSpace(resp.Data.CompanyName)
	}
	if name != "" {
		p.Name = name
	}
	if v := strings.TrimSpace(cd.EMail); v != "" {
		p.Email = v
	}
	if phones := companyPhones(cd.MobileNumber, cd.PhoneNumber); len(phones) > 0 {
		p.Phones = phones
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("livekeeping company has no name")
	}
	return s.st.UpsertBusinessProfile(ctx, p)
}

// syncGodowns pulls the business's godowns (physical locations) from Livekeeping
// and reconciles them into BusinessLocation rows, keyed by godownGuid. New
// godowns are inserted with a blank address for the owner to fill in; existing
// rows keep their owner-entered address (only the name is refreshed if
// Livekeeping renamed the godown); godowns that vanished from Livekeeping are
// pruned. Manually-added locations (blank source_id) are never touched.
func (s *LivekeepingService) syncGodowns(ctx context.Context, creds SyncCreds, res *SyncResult) error {
	// 1. Fetch every godown page (network only) before touching the DB.
	var godowns []godown
	start := 0
	for page := 0; page < livekeepingMaxPages; page++ {
		if page > 0 {
			if err := sleepCtx(ctx, livekeepingPageDelay); err != nil {
				return err
			}
		}
		resp, err := s.fetchGodowns(ctx, creds, start, livekeepingPageSize)
		if err != nil {
			return err
		}
		if len(resp.Data.List) == 0 {
			break
		}
		godowns = append(godowns, resp.Data.List...)
		start += len(resp.Data.List)
		if resp.Data.TotalCount > 0 && start >= resp.Data.TotalCount {
			break
		}
	}

	// 2. Load previously-synced locations keyed by SourceID (godownGuid) so we
	// can tell new godowns from renames and detect deletions. Manual locations
	// (blank source_id) are excluded and thus never pruned.
	existing := map[string]models.BusinessLocation{}
	var prev []models.BusinessLocation
	if err := s.db.WithContext(ctx).Where("source_id <> ''").Find(&prev).Error; err != nil {
		return fmt.Errorf("loading existing locations: %w", err)
	}
	for _, l := range prev {
		existing[l.SourceID] = l
	}

	// 3. Upsert each godown, preserving owner-entered addresses.
	seen := make(map[string]bool, len(godowns))
	for _, g := range godowns {
		guid := strings.TrimSpace(g.GodownGuid)
		name := strings.TrimSpace(g.GodownName)
		if guid == "" || name == "" {
			continue
		}
		seen[guid] = true

		if cur, ok := existing[guid]; ok {
			// Only refresh the name; never overwrite the owner's address.
			if cur.Name != name {
				if err := s.db.WithContext(ctx).Model(&models.BusinessLocation{}).
					Where("id = ?", cur.ID).Update("name", name).Error; err != nil {
					return err
				}
				res.LocationsUpdated++
			}
			continue
		}
		loc := models.BusinessLocation{Name: name, SourceID: guid, Active: true}
		if err := s.db.WithContext(ctx).Create(&loc).Error; err != nil {
			return err
		}
		res.LocationsCreated++
	}

	// 4. Prune previously-synced locations no longer present in Livekeeping.
	var stale []uint
	for guid, l := range existing {
		if !seen[guid] {
			stale = append(stale, l.ID)
		}
	}
	if len(stale) > 0 {
		if err := s.db.WithContext(ctx).Where("id IN ?", stale).Delete(&models.BusinessLocation{}).Error; err != nil {
			return fmt.Errorf("pruning removed locations: %w", err)
		}
		res.LocationsDeleted = len(stale)
	}
	return nil
}

// fetchGodowns fetches one page of godowns with the same bounded retry/backoff as
// the stock pages; auth/other 4xx return immediately.
func (s *LivekeepingService) fetchGodowns(ctx context.Context, creds SyncCreds, start, count int) (godownResponse, error) {
	var lastErr error
	for attempt := 0; attempt < livekeepingMaxAttempts; attempt++ {
		out, err := s.attemptGodowns(ctx, creds, start, count)
		if err == nil {
			return out, nil
		}
		re, ok := err.(*retryableError)
		if !ok {
			return out, err
		}
		lastErr = re.err
		if attempt == livekeepingMaxAttempts-1 {
			break
		}
		wait := livekeepingBaseBackoff << attempt
		if wait > livekeepingMaxBackoff {
			wait = livekeepingMaxBackoff
		}
		if re.retryAfter > wait {
			wait = re.retryAfter
		}
		if err := sleepCtx(ctx, wait); err != nil {
			return out, err
		}
	}
	return godownResponse{}, fmt.Errorf("livekeeping godowns still failing after %d attempts: %w", livekeepingMaxAttempts, lastErr)
}

// attemptGodowns performs a single getGodownsSummary request. The date range is
// the current Indian financial year (Apr 1 – Mar 31), which the endpoint requires.
func (s *LivekeepingService) attemptGodowns(ctx context.Context, creds SyncCreds, start, count int) (godownResponse, error) {
	var out godownResponse
	startDate, endDate := currentFinancialYear(time.Now())
	payload := map[string]interface{}{
		"_userId":     creds.UserID,
		"company_id":  creds.CompanyID,
		"startDate":   startDate,
		"endDate":     endDate,
		"startLimit":  start,
		"endLimit":    count,
		"sortBy":      "godownName",
		"searchTerm":  "",
		"requestFrom": "WEB",
		"orderBy":     1,
	}
	raw, err := s.sendLivekeeping(ctx, livekeepingGodownsURL, payload, creds.Token)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("could not parse livekeeping godowns response: %w", err)
	}
	if out.Status != 0 && (out.Status < 200 || out.Status >= 300) {
		msg := strings.TrimSpace(out.Message)
		if msg == "" {
			msg = fmt.Sprintf("status %d", out.Status)
		}
		return out, fmt.Errorf("livekeeping rejected the request: %s", msg)
	}
	return out, nil
}

// currentFinancialYear returns the Indian FY window containing now as
// "YYYY-04-01"/"YYYY-03-31" strings — the range getGodownsSummary expects.
func currentFinancialYear(now time.Time) (string, string) {
	y := now.Year()
	if now.Month() < time.April {
		y-- // Jan–Mar belongs to the FY that started the previous April
	}
	return fmt.Sprintf("%d-04-01", y), fmt.Sprintf("%d-03-31", y+1)
}

// companyPhones returns the distinct non-empty numbers from the mobile/phone fields.
func companyPhones(mobile, phone string) models.StringSlice {
	out := models.StringSlice{}
	mobile = strings.TrimSpace(mobile)
	phone = strings.TrimSpace(phone)
	if mobile != "" {
		out = append(out, mobile)
	}
	if phone != "" && phone != mobile {
		out = append(out, phone)
	}
	return out
}

// userIDFromToken extracts the Livekeeping user id from the session token's JWT
// payload (claim sub._userId), so callers don't have to store it separately.
// Returns "" if the token isn't a decodable JWT with that claim.
func userIDFromToken(token string) string {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		if raw, err = base64.URLEncoding.DecodeString(parts[1]); err != nil {
			return ""
		}
	}
	var claims struct {
		Sub struct {
			UserID string `json:"_userId"`
		} `json:"sub"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil {
		return ""
	}
	return strings.TrimSpace(claims.Sub.UserID)
}

// effectiveUserID prefers the user id embedded in the token, falling back to any
// stored/default id when the token can't be decoded.
func effectiveUserID(creds SyncCreds) string {
	if id := userIDFromToken(creds.Token); id != "" {
		return id
	}
	return creds.UserID
}

// parseRetryAfter reads a Retry-After header, which may be a number of seconds
// (e.g. "5") or an HTTP date. Returns 0 when absent/unparseable.
func parseRetryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// sleepCtx waits for d, but returns early if the context is cancelled (e.g. the
// owner navigated away or the request timed out).
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// itemSourceID returns the stable external id for a stock item — its Tally guid,
// or the numeric id as a fallback when the guid is blank.
func itemSourceID(it stockItem) string {
	if g := strings.TrimSpace(it.Guid); g != "" {
		return g
	}
	return strings.TrimSpace(it.ID)
}

// upsertOutcome reports what upsertItem did, so Sync can count created vs.
// updated vs. unchanged rows.
type upsertOutcome int

const (
	outcomeUnchanged upsertOutcome = iota
	outcomeCreated
	outcomeUpdated
)

// upsertItem maps one stock item to a Listing and either inserts it, updates the
// matching existing row (by SourceID) when its fields changed, or leaves an
// unchanged row alone. Its category is resolved from the Tally group.
func (s *LivekeepingService) upsertItem(ctx context.Context, it stockItem, existing map[string]models.Listing) (upsertOutcome, error) {
	name := strings.TrimSpace(it.StockItemName)
	if name == "" {
		return outcomeUnchanged, fmt.Errorf("item %s has no name", it.Guid)
	}
	sourceID := itemSourceID(it)

	// The item's Tally "group" (parent) is the most useful category grouping.
	catID, err := s.st.FindOrCreateCategory(ctx, it.Parent, "")
	if err != nil {
		return outcomeUnchanged, err
	}

	qty := int(math.Round(parseNum(it.ClosingBalance)))
	price := parseNum(it.AvgRate)
	data := models.JSONB{
		"opening_balance":      it.OpeningBalance,
		"closing_balance":      it.ClosingBalance,
		"closing_rate":         it.ClosingRate,
		"closing_value":        it.ClosingValue,
		"avg_rate":             it.AvgRate,
		"amount":               it.TotalAmount,
		"total_quantity":       it.TotalQuantity,
		"group":                it.Parent,
		"livekeeping_category": it.Category,
		"livekeeping_id":       it.ID,
		"guid":                 it.Guid,
	}

	listing := models.Listing{
		CategoryID: catID,
		Name:       name,
		HSNCode:    it.HsnCode,
		Unit:       it.BaseUnits,
		SourceID:   sourceID,
		Quantity:   &qty,
		Price:      &price,
		Data:       data,
		Active:     true,
	}

	prev, ok := existing[sourceID]
	if !ok || sourceID == "" {
		if err := s.db.WithContext(ctx).Create(&listing).Error; err != nil {
			return outcomeUnchanged, err
		}
		return outcomeCreated, nil
	}

	if !listingChanged(prev, listing) {
		return outcomeUnchanged, nil
	}
	// Update in place so the row's id (and any FKs to it) survive the sync.
	updates := map[string]interface{}{
		"category_id": catID,
		"name":        name,
		"hsn_code":    it.HsnCode,
		"unit":        it.BaseUnits,
		"quantity":    &qty,
		"price":       &price,
		"data":        data,
		"active":      true,
	}
	if err := s.db.WithContext(ctx).Model(&models.Listing{}).Where("id = ?", prev.ID).Updates(updates).Error; err != nil {
		return outcomeUnchanged, err
	}
	return outcomeUpdated, nil
}

// listingChanged reports whether the freshly-mapped item differs from the row we
// already have, so unchanged items skip a needless write.
func listingChanged(prev, next models.Listing) bool {
	if prev.CategoryID != next.CategoryID ||
		prev.Name != next.Name ||
		prev.HSNCode != next.HSNCode ||
		prev.Unit != next.Unit ||
		!intPtrEqual(prev.Quantity, next.Quantity) ||
		!floatPtrEqual(prev.Price, next.Price) {
		return true
	}
	// Data is volatile (balances, values); compare via canonical JSON. Go sorts
	// map keys when marshaling, so equal maps produce equal bytes.
	pb, _ := json.Marshal(prev.Data)
	nb, _ := json.Marshal(next.Data)
	return !bytes.Equal(pb, nb)
}

func intPtrEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func floatPtrEqual(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// parseNum parses a possibly-empty, possibly-signed numeric string. Livekeeping
// sends numbers as strings like "1678", "-60", "40174.89", "-0". Anything
// unparseable is treated as 0.
func parseNum(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}
