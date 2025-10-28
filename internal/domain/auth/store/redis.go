package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"xiaozhi-server-go/internal/domain/auth/model"

	"github.com/redis/go-redis/v9"
)

type redisStore struct {
	client *redis.Client
	ttl    time.Duration
	prefix string
}

// NewRedis constructs a redis-backed auth store.
func NewRedis(cfg Config) (Store, error) {
	if cfg.Redis == nil {
		return nil, fmt.Errorf("redis configuration missing")
	}
	if cfg.Redis.Addr == "" {
		return nil, fmt.Errorf("redis address required")
	}

	opts := &redis.Options{
		Addr:     cfg.Redis.Addr,
		Username: cfg.Redis.Username,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}
	client := redis.NewClient(opts)

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	prefix := cfg.Redis.Prefix
	if prefix == "" {
		prefix = "auth:client:"
	}

	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &redisStore{
		client: client,
		ttl:    ttl,
		prefix: prefix,
	}, nil
}

func (s *redisStore) key(id string) string {
	return s.prefix + id
}

func (s *redisStore) Store(ctx context.Context, info model.ClientInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	expiry := s.ttl
	if info.ExpiresAt != nil {
		expiry = time.Until(*info.ExpiresAt)
	}
	return s.client.Set(ctx, s.key(info.ClientID), data, expiry).Err()
}

func (s *redisStore) Validate(
	ctx context.Context,
	clientID string,
	username string,
	password string,
) (model.ClientInfo, bool, error) {
	info, err := s.Get(ctx, clientID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return model.ClientInfo{}, false, nil
		}
		return model.ClientInfo{}, false, err
	}
	if info.Username != username || info.Password != password {
		return model.ClientInfo{}, false, nil
	}
	return info, true, nil
}

func (s *redisStore) Get(ctx context.Context, clientID string) (model.ClientInfo, error) {
	raw, err := s.client.Get(ctx, s.key(clientID)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return model.ClientInfo{}, fmt.Errorf("client not found: %s", clientID)
		}
		return model.ClientInfo{}, err
	}
	var info model.ClientInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return model.ClientInfo{}, err
	}
	if info.ExpiresAt != nil && time.Now().After(*info.ExpiresAt) {
		_ = s.Remove(ctx, clientID)
		return model.ClientInfo{}, fmt.Errorf("client expired: %s", clientID)
	}
	return info, nil
}

func (s *redisStore) Remove(ctx context.Context, clientID string) error {
	return s.client.Del(ctx, s.key(clientID)).Err()
}

func (s *redisStore) List(ctx context.Context) ([]string, error) {
	var cursor uint64
	keys := make([]string, 0)
	pattern := s.prefix + "*"
	for {
		res, nextCursor, err := s.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range res {
			keys = append(keys, strings.TrimPrefix(key, s.prefix))
		}
		if nextCursor == 0 {
			break
		}
		cursor = nextCursor
	}
	return keys, nil
}

func (s *redisStore) CleanupExpired(context.Context) error {
	// Redis handles expiration via TTL.
	return nil
}

func (s *redisStore) Stats(ctx context.Context) (map[string]any, error) {
	size, err := s.client.DBSize(ctx).Result()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"type":  "redis",
		"total": size,
		"ttl":   int(s.ttl.Seconds()),
	}, nil
}

func (s *redisStore) Close(ctx context.Context) error {
	return s.client.Close()
}
