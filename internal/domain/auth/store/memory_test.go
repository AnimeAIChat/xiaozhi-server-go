package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"xiaozhi-server-go/internal/domain/auth/model"
)

func TestMemoryStoreBasicLifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewMemory(Config{
		TTL:    time.Second,
		Memory: &MemoryConfig{GCInterval: 10 * time.Millisecond},
	})
	t.Cleanup(func() {
		_ = store.Close(ctx)
	})

	info := model.ClientInfo{
		ClientID: "client-basic",
		Username: "user",
		Password: "pass",
		DeviceID: "device-1",
		IP:       "127.0.0.1",
		Metadata: map[string]any{"role": "tester"},
	}

	if err := store.Store(ctx, info); err != nil {
		t.Fatalf("Store returned error: %v", err)
	}

	stored, err := store.Get(ctx, info.ClientID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if stored.ClientID != info.ClientID || stored.Username != info.Username {
		t.Fatalf("unexpected client info: %+v", stored)
	}

	validated, ok, err := store.Validate(ctx, info.ClientID, info.Username, info.Password)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected validation success")
	}
	if validated.ClientID != info.ClientID {
		t.Fatalf("unexpected validation payload: %+v", validated)
	}

	ids, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(ids) != 1 || ids[0] != info.ClientID {
		t.Fatalf("expected list to include client: %v", ids)
	}

	if err := store.Remove(ctx, info.ClientID); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if _, err := store.Get(ctx, info.ClientID); err == nil {
		t.Fatalf("expected get error after removal")
	}
}

func TestMemoryStoreExpiration(t *testing.T) {
	ctx := context.Background()
	store := NewMemory(Config{
		TTL:    50 * time.Millisecond,
		Memory: &MemoryConfig{GCInterval: 5 * time.Millisecond},
	})
	t.Cleanup(func() {
		_ = store.Close(ctx)
	})

	info := model.ClientInfo{
		ClientID: "client-expire",
		Username: "user",
		Password: "pass",
	}
	if err := store.Store(ctx, info); err != nil {
		t.Fatalf("Store returned error: %v", err)
	}

	time.Sleep(80 * time.Millisecond)

	if err := store.CleanupExpired(ctx); err != nil {
		t.Fatalf("CleanupExpired returned error: %v", err)
	}

	if _, err := store.Get(ctx, info.ClientID); err == nil {
		t.Fatalf("expected get to fail after expiration")
	}

	if _, ok, err := store.Validate(ctx, info.ClientID, info.Username, info.Password); ok {
		t.Fatalf("expected validation to fail for expired entry")
	} else if err != nil && !strings.Contains(err.Error(), "expired") {
		t.Fatalf("unexpected validation error: %v", err)
	}

	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats returned error: %v", err)
	}
	if stats["active"].(int) != 0 {
		t.Fatalf("expected active count to be zero, got %v", stats["active"])
	}
}
