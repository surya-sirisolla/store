package db

import (
	"log"
	"store/backend/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// Open connects to Postgres without running migrations. Used by the MCP server,
// which must not race the API server's AutoMigrate on first boot.
func Open(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
}

// Connect opens the database and runs migrations. Used by the API server.
func Connect(dsn string) *gorm.DB {
	db, err := Open(dsn)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	// Legacy multi-tenant columns are dropped BEFORE AutoMigrate so leftover
	// NOT NULL / unique constraints on them can't reject inserts from the
	// single-tenant models.
	dropLegacyTenancy(db)

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
		log.Fatalf("migration failed: %v", err)
	}

	DB = db
	return db
}

// dropLegacyTenancy removes the schema left behind by the multi-tenant era.
// The product is now one business per deployment, but AutoMigrate never drops
// columns, indexes or tables, so an upgraded database would keep a stale
// business_id on every catalog table (plus the composite contacts index that
// blocks a plain unique phone). Every statement is IF EXISTS, so this is a
// no-op on a fresh install and safe to run on every boot.
func dropLegacyTenancy(db *gorm.DB) {
	stmts := []string{
		// The composite (business_id, phone) unique index must go before its
		// column, and before Contact's single-column unique index can be created.
		`DROP INDEX IF EXISTS idx_contacts_biz_phone`,
		`DROP INDEX IF EXISTS idx_contacts_phone`,
		// auth_credentials gained a username column. On installs that still have
		// the legacy shape (id/password_hash only, from the dead single-password
		// flow) AutoMigrate could not add a NOT NULL unique column to existing
		// rows, so drop that table and let seedAdmin re-create it from env. The
		// guard is essential: dropping unconditionally would reset the admin
		// password on every restart.
		`DO $$
		 BEGIN
		   IF EXISTS (SELECT 1 FROM information_schema.tables
		              WHERE table_schema = 'public' AND table_name = 'auth_credentials')
		      AND NOT EXISTS (SELECT 1 FROM information_schema.columns
		                      WHERE table_schema = 'public' AND table_name = 'auth_credentials'
		                        AND column_name = 'username')
		   THEN
		     DROP TABLE auth_credentials;
		   END IF;
		 END $$`,
		// Login accounts and tenants: the console now has one admin credential
		// (auth_credentials) and users is purely the staff phone registry.
		`ALTER TABLE IF EXISTS users DROP COLUMN IF EXISTS business_id`,
		`ALTER TABLE IF EXISTS users DROP COLUMN IF EXISTS role`,
		`ALTER TABLE IF EXISTS users DROP COLUMN IF EXISTS password_hash`,
		`DROP TABLE IF EXISTS businesses`,
	}
	for _, t := range []string{
		"business_profiles", "business_locations", "categories", "listings",
		"alert_requests", "bot_activities", "contacts", "bulk_upload_jobs",
	} {
		stmts = append(stmts, `ALTER TABLE IF EXISTS `+t+` DROP COLUMN IF EXISTS business_id`)
	}

	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			// Non-fatal: a fresh database has none of this, and a partial drop
			// shouldn't stop the server from coming up.
			log.Printf("dropping legacy tenancy (%s): %v", stmt, err)
		}
	}
}
