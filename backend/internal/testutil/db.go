// Package testutil provides shared helpers for backend tests.
package testutil

import (
	"fmt"
	"os"
	"testing"

	"store/backend/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewDB connects to the test database, migrates the schema, and truncates all
// tables so each test starts from a clean slate. If the test database is
// unreachable, the calling test is skipped rather than failed — tests stay
// runnable on machines without the DB up, but run for real when it is.
//
// Override connection settings with TEST_DB_* env vars; defaults match the
// docker-compose Postgres with a dedicated `storedb_test` database.
func NewDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
		env("TEST_DB_HOST", "localhost"),
		env("TEST_DB_PORT", "5432"),
		env("TEST_DB_USER", "storeuser"),
		env("TEST_DB_PASSWORD", "storepass"),
		env("TEST_DB_NAME", "storedb_test"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Skipf("test database unavailable (%v); skipping — bring it up with docker compose + CREATE DATABASE storedb_test", err)
	}

	if err := db.AutoMigrate(
		&models.User{},
		&models.AuthCredential{},
		&models.BusinessProfile{},
		&models.BusinessLocation{},
		&models.Category{},
		&models.Listing{},
		&models.AlertRequest{},
		&models.BotActivity{},
		&models.BulkUploadJob{},
		&models.Contact{},
		&models.ScheduledJob{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	truncateAll(t, db)
	t.Cleanup(func() { truncateAll(t, db) })
	return db
}

func truncateAll(t *testing.T, db *gorm.DB) {
	t.Helper()
	// Order respects nothing thanks to CASCADE + RESTART IDENTITY.
	err := db.Exec(`TRUNCATE TABLE
		users, auth_credentials, business_profiles, business_locations,
		categories, listings, alert_requests, bot_activities,
		bulk_upload_jobs, contacts, scheduled_jobs
		RESTART IDENTITY CASCADE`).Error
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
