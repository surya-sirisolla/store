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

	if err := db.AutoMigrate(
		&models.User{},
		&models.BusinessProfile{},
		&models.Category{},
		&models.Listing{},
		&models.AlertRequest{},
		&models.BotActivity{},
		&models.BulkUploadJob{},
		&models.Contact{},
	); err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	// AutoMigrate only adds columns, it never drops or relaxes ones removed
	// from a model. password_hash predates the no-login redesign and still
	// carries a NOT NULL constraint on installs upgraded from that version,
	// which would block every new User insert. Safe no-op on fresh installs.
	if err := db.Exec(`ALTER TABLE users DROP COLUMN IF EXISTS password_hash`).Error; err != nil {
		log.Printf("drop legacy password_hash column: %v", err)
	}

	DB = db
	return db
}
