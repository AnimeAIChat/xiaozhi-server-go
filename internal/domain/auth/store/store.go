package store

import (
	"context"
	"time"

	"xiaozhi-server-go/internal/domain/auth/model"
)

// Store defines the behaviour required by the auth manager.
type Store interface {
	Store(ctx context.Context, info model.ClientInfo) error
	Validate(ctx context.Context, clientID, username, password string) (model.ClientInfo, bool, error)
	Get(ctx context.Context, clientID string) (model.ClientInfo, error)
	Remove(ctx context.Context, clientID string) error
	List(ctx context.Context) ([]string, error)
	CleanupExpired(ctx context.Context) error
	Stats(ctx context.Context) (map[string]any, error)
	Close(ctx context.Context) error
}

// Config describes the high level store selection parameters.
type Config struct {
	Driver          string
	TTL             time.Duration
	Namespace       string
	Redis           *RedisConfig
	SQLite          *SQLiteConfig
	Memory          *MemoryConfig
	BackgroundClean bool
}

// MemoryConfig holds in-memory tuning knobs.
type MemoryConfig struct {
	GCInterval time.Duration
}

// SQLiteConfig provides the database dependency.
type SQLiteConfig struct {
	DSN string
}

// RedisConfig captures connection options.
type RedisConfig struct {
	Addr     string
	Username string
	Password string
	DB       int
	Prefix   string
}
