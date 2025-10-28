package store

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"xiaozhi-server-go/internal/domain/auth/model"
	"xiaozhi-server-go/src/models"
)

func TestFactoryMemory(t *testing.T) {
	store, err := New(Config{Driver: DriverMemory}, Dependencies{})
	if err != nil {
		t.Fatalf("New memory store: %v", err)
	}
	defer store.Close(context.Background())
}

func TestFactorySQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.AuthClient{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	store, err := New(Config{
		Driver: DriverSQLite,
		TTL:    time.Second,
	}, Dependencies{SQLiteDB: db})
	if err != nil {
		t.Fatalf("New sqlite store: %v", err)
	}
	defer store.Close(context.Background())

	if err := store.Store(context.Background(), model.ClientInfo{ClientID: "factory-sqlite"}); err != nil {
		t.Fatalf("Store error: %v", err)
	}
}

func TestFactoryRedis(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	store, err := New(Config{
		Driver: DriverRedis,
		TTL:    time.Second,
		Redis: &RedisConfig{
			Addr: mr.Addr(),
		},
	}, Dependencies{})
	if err != nil {
		t.Fatalf("New redis store: %v", err)
	}
	defer store.Close(context.Background())

	if err := store.Store(context.Background(), model.ClientInfo{ClientID: "factory-redis"}); err != nil {
		t.Fatalf("Store error: %v", err)
	}
}

func TestFactoryUnsupported(t *testing.T) {
	if _, err := New(Config{Driver: "unknown"}, Dependencies{}); err == nil {
		t.Fatalf("expected error for unsupported driver")
	}
}
