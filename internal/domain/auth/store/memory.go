package store

import (
	"context"
	"fmt"
	"sync"
	"time"

	"xiaozhi-server-go/internal/domain/auth/model"
)

type memoryStore struct {
	items       map[string]model.ClientInfo
	mutex       sync.RWMutex
	ttl         time.Duration
	cleanupFreq time.Duration
	stop        chan struct{}
	stopOnce    sync.Once
}

// NewMemory builds an in-memory auth store.
func NewMemory(cfg Config) Store {
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	cleanup := 5 * time.Minute
	if cfg.Memory != nil && cfg.Memory.GCInterval > 0 {
		cleanup = cfg.Memory.GCInterval
	}
	s := &memoryStore{
		items:       make(map[string]model.ClientInfo),
		ttl:         ttl,
		cleanupFreq: cleanup,
		stop:        make(chan struct{}),
	}
	go s.gcLoop()
	return s
}

func (s *memoryStore) gcLoop() {
	ticker := time.NewTicker(s.cleanupFreq)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = s.CleanupExpired(context.Background())
		case <-s.stop:
			return
		}
	}
}

func (s *memoryStore) Store(_ context.Context, info model.ClientInfo) error {
	if info.ClientID == "" {
		return fmt.Errorf("client id required")
	}
	now := time.Now()
	if info.CreatedAt.IsZero() {
		info.CreatedAt = now
	}
	if info.ExpiresAt == nil && s.ttl > 0 {
		exp := now.Add(s.ttl)
		info.ExpiresAt = &exp
	}

	s.mutex.Lock()
	s.items[info.ClientID] = info
	s.mutex.Unlock()
	return nil
}

func (s *memoryStore) Validate(
	_ context.Context,
	clientID string,
	username string,
	password string,
) (model.ClientInfo, bool, error) {
	s.mutex.RLock()
	info, ok := s.items[clientID]
	s.mutex.RUnlock()
	if !ok {
		return model.ClientInfo{}, false, nil
	}
	if info.ExpiresAt != nil && time.Now().After(*info.ExpiresAt) {
		return model.ClientInfo{}, false, fmt.Errorf("expired credentials")
	}
	if info.Username != username || info.Password != password {
		return model.ClientInfo{}, false, nil
	}
	return info, true, nil
}

func (s *memoryStore) Get(_ context.Context, clientID string) (model.ClientInfo, error) {
	s.mutex.RLock()
	info, ok := s.items[clientID]
	s.mutex.RUnlock()
	if !ok {
		return model.ClientInfo{}, fmt.Errorf("client not found: %s", clientID)
	}
	if info.ExpiresAt != nil && time.Now().After(*info.ExpiresAt) {
		return model.ClientInfo{}, fmt.Errorf("client expired: %s", clientID)
	}
	return info, nil
}

func (s *memoryStore) Remove(_ context.Context, clientID string) error {
	s.mutex.Lock()
	delete(s.items, clientID)
	s.mutex.Unlock()
	return nil
}

func (s *memoryStore) List(_ context.Context) ([]string, error) {
	now := time.Now()
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	ids := make([]string, 0, len(s.items))
	for id, item := range s.items {
		if item.ExpiresAt == nil || now.Before(*item.ExpiresAt) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (s *memoryStore) CleanupExpired(_ context.Context) error {
	now := time.Now()
	s.mutex.Lock()
	for id, item := range s.items {
		if item.ExpiresAt != nil && now.After(*item.ExpiresAt) {
			delete(s.items, id)
		}
	}
	s.mutex.Unlock()
	return nil
}

func (s *memoryStore) Stats(_ context.Context) (map[string]any, error) {
	now := time.Now()
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	total := len(s.items)
	active := 0
	for _, info := range s.items {
		if info.ExpiresAt == nil || now.Before(*info.ExpiresAt) {
			active++
		}
	}
	return map[string]any{
		"type":        "memory",
		"total":       total,
		"active":      active,
		"ttl_seconds": int(s.ttl.Seconds()),
	}, nil
}

func (s *memoryStore) Close(_ context.Context) error {
	s.stopOnce.Do(func() {
		close(s.stop)
	})
	return nil
}
