// Package services — jobs.go is a small in-process scheduler for the console's
// Automation section. It owns the ScheduledJob rows, runs jobs on their schedule
// (or on demand), and records each outcome. Registered jobs are the Livekeeping
// stock sync and the idle-chat cleanup; the machinery is job-key based so more
// can be added.
package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"store/backend/internal/models"
	"store/backend/internal/secrets"
	"store/backend/internal/store"

	"gorm.io/gorm"
)

const (
	jobLivekeepingSync = "livekeeping_sync"
	jobSessionCleanup  = "session_cleanup"
)

// Per-run timeouts. Syncs/cleanup are quick; a throttled broadcast to a full
// contact list can legitimately take a long time, so it gets a generous ceiling.
const (
	syncRunTimeout      = 15 * time.Minute
	broadcastRunTimeout = 3 * time.Hour
)

// JobService runs and schedules the console's background jobs.
type JobService struct {
	db        *gorm.DB
	st        *store.Store
	lk        *LivekeepingService
	broadcast *BroadcastService
	credFile  string // path to the Livekeeping creds JSON on the shared volume

	// sessionsDir is the bot's per-chat conversation store (picoclaw's
	// <workspace>/sessions), shared into this container so the cleanup job can
	// prune it. idleHours is how long a chat must be untouched before it's cleared.
	sessionsDir string
	idleHours   int

	mu      sync.Mutex
	running map[string]bool // job key -> currently executing
}

// jobSeed is the default row for a known job on first boot.
type jobSeed struct {
	Key           string
	Name          string
	Enabled       bool
	ScheduleKind  string
	IntervalHours int
	DailyTime     string
}

var jobSeeds = []jobSeed{
	{jobLivekeepingSync, "Livekeeping stock sync", false, "manual", 6, "02:00"},
	// Idle-chat cleanup runs daily by default so stale (and any corrupted)
	// conversation memory can't linger and replay.
	{jobSessionCleanup, "Reset idle chats", true, "daily", 24, "04:00"},
}

// NewJobService wires the runner and makes sure each known job has a row.
func NewJobService(db *gorm.DB, st *store.Store, lk *LivekeepingService, broadcast *BroadcastService, credFile, sessionsDir string, idleHours int) *JobService {
	if idleHours < 1 {
		idleHours = 24
	}
	s := &JobService{
		db: db, st: st, lk: lk, broadcast: broadcast, credFile: credFile,
		sessionsDir: sessionsDir, idleHours: idleHours,
		running: map[string]bool{},
	}
	s.ensureSeed()
	return s
}

// ensureSeed creates the row for each known job on first boot. Existing rows
// (with the owner's schedule) are left as-is.
func (s *JobService) ensureSeed() {
	for _, seed := range jobSeeds {
		var count int64
		s.db.Model(&models.ScheduledJob{}).Where("key = ?", seed.Key).Count(&count)
		if count > 0 {
			continue
		}
		job := models.ScheduledJob{
			Key:           seed.Key,
			Name:          seed.Name,
			Enabled:       seed.Enabled,
			ScheduleKind:  seed.ScheduleKind,
			IntervalHours: seed.IntervalHours,
			DailyTime:     seed.DailyTime,
			LastStatus:    "idle",
		}
		job.NextRunAt = nextRun(job, time.Now().UTC())
		if err := s.db.Create(&job).Error; err != nil {
			log.Printf("seed scheduled job %s: %v", seed.Key, err)
		}
	}
}

// List returns every job with its current schedule + last outcome.
func (s *JobService) List() ([]models.ScheduledJob, error) {
	var jobs []models.ScheduledJob
	err := s.db.Order("id").Find(&jobs).Error
	return jobs, err
}

func (s *JobService) get(key string) (models.ScheduledJob, error) {
	var job models.ScheduledJob
	err := s.db.Where("key = ?", key).First(&job).Error
	return job, err
}

// ScheduleInput is the owner-settable schedule for a job.
type ScheduleInput struct {
	Enabled       bool   `json:"enabled"`
	ScheduleKind  string `json:"schedule_kind"` // manual | interval | daily
	IntervalHours int    `json:"interval_hours"`
	DailyTime     string `json:"daily_time"` // "HH:MM"
}

// SaveSchedule validates and persists a job's schedule, recomputing its next run.
func (s *JobService) SaveSchedule(key string, in ScheduleInput) (models.ScheduledJob, error) {
	job, err := s.get(key)
	if err != nil {
		return job, err
	}

	kind, err := validateScheduleKind(in)
	if err != nil {
		return job, err
	}

	job.Enabled = in.Enabled
	job.ScheduleKind = kind
	if in.IntervalHours >= 1 {
		job.IntervalHours = in.IntervalHours
	}
	if validDailyTime(in.DailyTime) {
		job.DailyTime = in.DailyTime
	}
	job.NextRunAt = nextRun(job, time.Now().UTC())

	if err := s.db.Save(&job).Error; err != nil {
		return job, err
	}
	return job, nil
}

// validateScheduleKind normalizes and validates a schedule input, returning the
// canonical kind or an error the owner can act on.
func validateScheduleKind(in ScheduleInput) (string, error) {
	kind := strings.TrimSpace(in.ScheduleKind)
	switch kind {
	case "manual", "interval", "daily":
	case "":
		kind = "manual"
	default:
		return "", fmt.Errorf("unknown schedule kind %q", in.ScheduleKind)
	}
	if kind == "interval" && in.IntervalHours < 1 {
		return "", errors.New("interval must be at least 1 hour")
	}
	if kind == "daily" && !validDailyTime(in.DailyTime) {
		return "", errors.New("daily time must be HH:MM (24-hour)")
	}
	return kind, nil
}

func normalizeInterval(h int) int {
	if h < 1 {
		return 6
	}
	return h
}

func normalizeDaily(s string) string {
	if validDailyTime(s) {
		return s
	}
	return "02:00"
}

// BroadcastInput is the owner-submitted payload to create or edit a broadcast job.
type BroadcastInput struct {
	Name     string          `json:"name"`
	Schedule ScheduleInput   `json:"schedule"`
	Config   BroadcastConfig `json:"config"`
}

// PreviewBroadcast returns the composed message body and recipient count for a
// prospective broadcast, without sending — powers the create/edit form preview.
func (s *JobService) PreviewBroadcast(ctx context.Context, cfg BroadcastConfig) (string, int, error) {
	return s.broadcast.Preview(ctx, parseBroadcastConfig(configToJSONB(cfg)))
}

// CreateBroadcast creates an owner-defined reminder job.
func (s *JobService) CreateBroadcast(in BroadcastInput) (models.ScheduledJob, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return models.ScheduledJob{}, errors.New("give the reminder a name")
	}
	kind, err := validateScheduleKind(in.Schedule)
	if err != nil {
		return models.ScheduledJob{}, err
	}
	job := models.ScheduledJob{
		Key:           "broadcast_" + randomKey(),
		Name:          name,
		Type:          "broadcast",
		Deletable:     true,
		Config:        configToJSONB(in.Config),
		Enabled:       in.Schedule.Enabled,
		ScheduleKind:  kind,
		IntervalHours: normalizeInterval(in.Schedule.IntervalHours),
		DailyTime:     normalizeDaily(in.Schedule.DailyTime),
		LastStatus:    "idle",
	}
	job.NextRunAt = nextRun(job, time.Now().UTC())
	if err := s.db.Create(&job).Error; err != nil {
		return models.ScheduledJob{}, err
	}
	return job, nil
}

// UpdateBroadcast edits an existing broadcast job's name, message and schedule.
func (s *JobService) UpdateBroadcast(key string, in BroadcastInput) (models.ScheduledJob, error) {
	job, err := s.get(key)
	if err != nil {
		return job, err
	}
	if job.Type != "broadcast" {
		return job, errors.New("only reminder jobs can be edited")
	}
	kind, err := validateScheduleKind(in.Schedule)
	if err != nil {
		return job, err
	}
	if n := strings.TrimSpace(in.Name); n != "" {
		job.Name = n
	}
	job.Config = configToJSONB(in.Config)
	job.Enabled = in.Schedule.Enabled
	job.ScheduleKind = kind
	job.IntervalHours = normalizeInterval(in.Schedule.IntervalHours)
	job.DailyTime = normalizeDaily(in.Schedule.DailyTime)
	job.NextRunAt = nextRun(job, time.Now().UTC())
	if err := s.db.Save(&job).Error; err != nil {
		return job, err
	}
	return job, nil
}

// Delete removes an owner-created job. System jobs cannot be deleted.
func (s *JobService) Delete(key string) error {
	job, err := s.get(key)
	if err != nil {
		return err
	}
	if !job.Deletable {
		return errors.New("this is a built-in job and can't be deleted")
	}
	return s.db.Delete(&models.ScheduledJob{}, job.ID).Error
}

func configToJSONB(cfg BroadcastConfig) models.JSONB {
	b, _ := json.Marshal(cfg)
	var out models.JSONB
	_ = json.Unmarshal(b, &out)
	return out
}

func randomKey() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// RunNow triggers a job immediately in the background. It's a no-op error if the
// job is already running (so the owner can't stack duplicate syncs).
func (s *JobService) RunNow(key string) error {
	if _, err := s.get(key); err != nil {
		return err
	}
	s.mu.Lock()
	if s.running[key] {
		s.mu.Unlock()
		return errors.New("this job is already running")
	}
	s.running[key] = true
	s.mu.Unlock()

	s.db.Model(&models.ScheduledJob{}).Where("key = ?", key).
		Updates(map[string]interface{}{"last_status": "running", "last_message": ""})

	go s.execute(key)
	return nil
}

// execute runs a job to completion and records the outcome. Always releases the
// running flag.
func (s *JobService) execute(key string) {
	defer func() {
		s.mu.Lock()
		delete(s.running, key)
		s.mu.Unlock()
	}()

	job, _ := s.get(key)

	timeout := syncRunTimeout
	if job.Type == "broadcast" {
		timeout = broadcastRunTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var (
		result  models.JSONB
		message string
		runErr  error
	)
	switch {
	case key == jobLivekeepingSync:
		result, message, runErr = s.runLivekeeping(ctx)
	case key == jobSessionCleanup:
		result, message, runErr = s.runSessionCleanup()
	case job.Type == "broadcast":
		result, message, runErr = s.broadcast.Run(ctx, parseBroadcastConfig(job.Config))
	default:
		runErr = fmt.Errorf("unknown job %q", key)
	}

	now := time.Now().UTC()
	updates := map[string]interface{}{
		"last_run_at": now,
		"last_result": result,
	}
	if runErr != nil {
		updates["last_status"] = "failed"
		updates["last_message"] = runErr.Error()
	} else {
		updates["last_status"] = "success"
		updates["last_message"] = message
	}
	// Schedule the next run from now, so a long job doesn't immediately re-fire.
	if job, err := s.get(key); err == nil {
		updates["next_run_at"] = nextRun(job, now)
	}
	s.db.Model(&models.ScheduledJob{}).Where("key = ?", key).Updates(updates)
}

// runLivekeeping performs the stock sync with the saved credentials and keeps the
// cached token-validity flag in sync with the outcome.
func (s *JobService) runLivekeeping(ctx context.Context) (models.JSONB, string, error) {
	cfg := secrets.ReadLivekeeping(s.credFile)
	if !cfg.Configured() {
		return nil, "", errors.New("paste your Livekeeping token first")
	}

	scope := SyncScope{
		Stock:   cfg.StockEnabled(),
		Godowns: cfg.GodownsEnabled(),
		Profile: cfg.ProfileEnabled(),
	}
	if !scope.Stock && !scope.Godowns && !scope.Profile {
		return nil, "", errors.New("nothing selected to sync — enable at least one item under Extensions")
	}

	res, err := s.lk.Sync(ctx, SyncCreds{Token: cfg.Token, CompanyID: cfg.CompanyID, UserID: cfg.UserID}, scope)
	if err != nil {
		var authErr *AuthError
		if errors.As(err, &authErr) {
			no := false
			cfg.TokenValid = &no
			cfg.TokenCheckedAt = time.Now().UTC().Format(time.RFC3339)
			cfg.TokenError = err.Error()
			_ = secrets.WriteLivekeeping(s.credFile, cfg)
		}
		return nil, "", err
	}

	yes := true
	cfg.TokenValid = &yes
	cfg.TokenCheckedAt = time.Now().UTC().Format(time.RFC3339)
	cfg.TokenError = ""
	cfg.LastSyncAt = time.Now().UTC().Format(time.RFC3339)
	cfg.LastSyncCount = res.Fetched
	_ = secrets.WriteLivekeeping(s.credFile, cfg)

	// Freshly-synced stock may satisfy waitlisted customers — raise their alerts.
	raised := 0
	if scope.Stock {
		raised = s.st.RaiseReadyAlerts(ctx)
	}

	// Build a message from only the parts that actually ran.
	parts := []string{}
	if scope.Stock {
		p := fmt.Sprintf("Imported %d of %d items (%d new, %d updated, %d removed)",
			res.Fetched, res.Total, res.Created, res.Updated, res.Deleted)
		if res.Errors > 0 {
			p += fmt.Sprintf(", %d skipped", res.Errors)
		}
		parts = append(parts, p)
	}
	if scope.Profile && res.CompanyUpdated {
		parts = append(parts, "business profile refreshed")
	}
	if scope.Godowns {
		parts = append(parts, fmt.Sprintf("locations: %d new, %d updated, %d removed",
			res.LocationsCreated, res.LocationsUpdated, res.LocationsDeleted))
	}
	msg := strings.Join(parts, " · ")
	if msg == "" {
		msg = "Nothing to sync."
	}
	if raised > 0 {
		msg += fmt.Sprintf(" — %d waitlist alert(s) now ready to notify", raised)
	}
	result := models.JSONB{
		"total": res.Total, "fetched": res.Fetched, "created": res.Created,
		"updated": res.Updated, "deleted": res.Deleted, "errors": res.Errors,
		"locations_created": res.LocationsCreated,
		"locations_updated": res.LocationsUpdated,
		"locations_deleted": res.LocationsDeleted,
		// What actually changed (names, capped) so the console can show detail.
		"created_items": res.CreatedItems,
		"updated_items": res.UpdatedItems,
		"deleted_items": res.DeletedItems,
	}
	return result, msg, nil
}

// runSessionCleanup deletes the bot's conversation-memory files for chats that
// have been idle longer than idleHours, so stale (or corrupted) short-term
// memory can't linger and replay. It only touches the sessions directory — the
// WhatsApp pairing and everything else are left alone. A returning customer
// simply starts a fresh conversation; durable business data lives in Postgres.
func (s *JobService) runSessionCleanup() (models.JSONB, string, error) {
	dir := strings.TrimSpace(s.sessionsDir)
	if dir == "" {
		return nil, "", errors.New("sessions directory is not configured")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// No sessions yet (bot hasn't been messaged) — nothing to do.
			return models.JSONB{"deleted": 0, "kept": 0}, "No chat history to clean up yet.", nil
		}
		return nil, "", fmt.Errorf("reading sessions: %w", err)
	}

	cutoff := time.Now().Add(-time.Duration(s.idleHours) * time.Hour)
	deleted, kept, errCount := 0, 0, 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Session state is stored as "<key>.jsonl" plus a "<key>.meta.json" sidecar.
		if !strings.HasSuffix(name, ".jsonl") && !strings.HasSuffix(name, ".meta.json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			if strings.HasSuffix(name, ".jsonl") {
				kept++
			}
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			errCount++
			continue
		}
		if strings.HasSuffix(name, ".jsonl") {
			deleted++
		}
	}

	msg := fmt.Sprintf("Cleared %d idle chat(s) (untouched > %dh); %d still active.", deleted, s.idleHours, kept)
	if errCount > 0 {
		msg += fmt.Sprintf(" %d file(s) could not be removed.", errCount)
	}
	return models.JSONB{"deleted": deleted, "kept": kept, "errors": errCount}, msg, nil
}

// Start launches the background scheduler. It ticks once a minute and fires any
// enabled job whose next run is due (and isn't already running).
func (s *JobService) Start() {
	// A row still marked "running" means a run was interrupted by a restart —
	// no goroutine is tracking it anymore, so clear the stuck state.
	s.db.Model(&models.ScheduledJob{}).Where("last_status = ?", "running").
		Updates(map[string]interface{}{"last_status": "failed", "last_message": "interrupted by a restart"})
	// Make sure every enabled job has a next-run time on boot (e.g. after a
	// restart that cleared an in-memory-only schedule).
	s.primeNextRuns()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s.tick()
		}
	}()
}

func (s *JobService) primeNextRuns() {
	jobs, err := s.List()
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for _, job := range jobs {
		if job.Enabled && job.ScheduleKind != "manual" && job.NextRunAt == nil {
			next := nextRun(job, now)
			s.db.Model(&models.ScheduledJob{}).Where("id = ?", job.ID).Update("next_run_at", next)
		}
	}
}

func (s *JobService) tick() {
	jobs, err := s.List()
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for _, job := range jobs {
		if !job.Enabled || job.ScheduleKind == "manual" || job.NextRunAt == nil {
			continue
		}
		if now.Before(*job.NextRunAt) {
			continue
		}
		if err := s.RunNow(job.Key); err != nil {
			// Already running or gone — try again on the next tick.
			continue
		}
	}
}

// nextRun computes when a job should run next, or nil for manual/disabled jobs.
func nextRun(job models.ScheduledJob, from time.Time) *time.Time {
	if !job.Enabled {
		return nil
	}
	switch job.ScheduleKind {
	case "interval":
		h := job.IntervalHours
		if h < 1 {
			h = 1
		}
		t := from.Add(time.Duration(h) * time.Hour)
		return &t
	case "daily":
		hh, mm, ok := parseDailyTime(job.DailyTime)
		if !ok {
			return nil
		}
		t := time.Date(from.Year(), from.Month(), from.Day(), hh, mm, 0, 0, time.UTC)
		if !t.After(from) {
			t = t.Add(24 * time.Hour)
		}
		return &t
	default:
		return nil
	}
}

func validDailyTime(s string) bool {
	_, _, ok := parseDailyTime(s)
	return ok
}

func parseDailyTime(s string) (int, int, bool) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, 0, false
	}
	return t.Hour(), t.Minute(), true
}
