package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"xiaozhi-server-go/internal/domain/auth/model"
	"xiaozhi-server-go/src/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:test-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.AuthClient{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func TestSQLiteStoreLifecycle(t *testing.T) {
	ctx := context.Background()
	db := newTestSQLiteDB(t)

	store, err := NewSQLite(db, Config{TTL: time.Second})
	if err != nil {
		t.Fatalf("NewSQLite error: %v", err)
	}

	info := model.ClientInfo{
		ClientID: "sqlite-client",
		Username: "user",
		Password: "pass",
		Metadata: map[string]any{"level": 1},
	}

	if err := store.Store(ctx, info); err != nil {
		t.Fatalf("Store error: %v", err)
	}

	got, err := store.Get(ctx, info.ClientID)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.ClientID != info.ClientID || got.Username != info.Username {
		t.Fatalf("unexpected client info: %+v", got)
	}

	_, ok, err := store.Validate(ctx, info.ClientID, info.Username, info.Password)
	if err != nil {
		t.Fatalf("Validate error: %v", err)
	}
	if !ok {
		t.Fatalf("expected validation to succeed")
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(list) != 1 || list[0] != info.ClientID {
		t.Fatalf("unexpected list: %v", list)
	}

	if err := store.Remove(ctx, info.ClientID); err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	if _, err := store.Get(ctx, info.ClientID); err == nil {
		t.Fatalf("expected missing after removal")
	}
}

func TestSQLiteStoreCleanup(t *testing.T) {
	ctx := context.Background()
	db := newTestSQLiteDB(t)

	store, err := NewSQLite(db, Config{TTL: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewSQLite error: %v", err)
	}

	now := time.Now()
	expired := now.Add(-time.Minute)
	info := model.ClientInfo{
		ClientID:  "expired-sqlite",
		Username:  "user",
		Password:  "pass",
		CreatedAt: now.Add(-time.Hour),
		ExpiresAt: &expired,
	}

	if err := store.Store(ctx, info); err != nil {
		t.Fatalf("Store error: %v", err)
	}

	if err := store.CleanupExpired(ctx); err != nil {
		t.Fatalf("CleanupExpired error: %v", err)
	}

	if _, err := store.Get(ctx, info.ClientID); err == nil {
		t.Fatalf("expected get to fail for expired client")
	}
}
