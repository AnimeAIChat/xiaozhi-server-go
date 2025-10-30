package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitConfigStore ensures the underlying configuration store is ready.
// Since we no longer use database-backed configuration, this is a no-op.
func InitConfigStore() error {
	return nil
}

// ConfigStore returns the default configuration store implementation.
// Since we no longer use database-backed configuration, this returns nil.
func ConfigStore() interface{} {
	return nil
}

// Global database instance for backward compatibility
var db *gorm.DB

// InitDatabase initializes the SQLite database for authentication and other services.
func InitDatabase() error {
	if db != nil {
		return nil
	}

	// Create data directory if it doesn't exist
	dataDir := "./data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, "xiaozhi.db")

	var err error
	db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Auto-migrate tables
	if err := db.AutoMigrate(&AuthClient{}); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	return nil
}

// GetDB returns the global database instance.
func GetDB() *gorm.DB {
	if db == nil {
		panic("database not initialized, call InitDatabase() first")
	}
	return db
}

// AuthClient represents the authentication client model for GORM
type AuthClient struct {
	ClientID  string `gorm:"primaryKey"`
	Username  string
	Password  string
	IP        string
	DeviceID  string
	CreatedAt *time.Time
	ExpiresAt *time.Time
	Metadata  []byte
}
