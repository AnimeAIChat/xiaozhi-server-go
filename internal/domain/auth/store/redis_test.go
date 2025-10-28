package store

import (
	"context"
	"testing"
	"time"

	"xiaozhi-server-go/internal/domain/auth/model"

	miniredis "github.com/alicebob/miniredis/v2"
)

func TestRedisStoreLifecycle(t *testing.T) {
	ctx := context.Background()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	store, err := NewRedis(Config{
		TTL: time.Second,
		Redis: &RedisConfig{
			Addr: mr.Addr(),
		},
	})
	if err != nil {
		t.Fatalf("NewRedis error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close(ctx)
	})

	info := model.ClientInfo{
		ClientID: "redis-client",
		Username: "user",
		Password: "pass",
	}
	if err := store.Store(ctx, info); err != nil {
		t.Fatalf("Store error: %v", err)
	}

	got, err := store.Get(ctx, info.ClientID)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.ClientID != info.ClientID {
		t.Fatalf("unexpected client: %+v", got)
	}

	_, ok, err := store.Validate(ctx, info.ClientID, info.Username, info.Password)
	if err != nil {
		t.Fatalf("Validate error: %v", err)
	}
	if !ok {
		t.Fatalf("expected validation success")
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
		t.Fatalf("expected missing client after removal")
	}
}
