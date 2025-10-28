package store

import (
	"fmt"

	"gorm.io/gorm"
)

// Driver identifiers supported by the auth domain.
const (
	DriverMemory = "memory"
	DriverSQLite = "sqlite"
	DriverRedis  = "redis"
)

// Dependencies captures external handles required by certain drivers.
type Dependencies struct {
	SQLiteDB *gorm.DB
}

// New creates an auth store based on the provided configuration.
func New(cfg Config, deps Dependencies) (Store, error) {
	driver := cfg.Driver
	if driver == "" {
		driver = DriverMemory
	}

	switch driver {
	case DriverMemory:
		return NewMemory(cfg), nil
	case DriverSQLite:
		if deps.SQLiteDB == nil {
			return nil, fmt.Errorf("sqlite driver requires database handle")
		}
		return NewSQLite(deps.SQLiteDB, cfg)
	case DriverRedis:
		return NewRedis(cfg)
	default:
		return nil, fmt.Errorf("unsupported auth store driver: %s", driver)
	}
}
