package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"xiaozhi-server-go/internal/domain/auth/model"
	"xiaozhi-server-go/internal/platform/storage"

	"gorm.io/gorm"
)

type sqliteStore struct {
	db  *gorm.DB
	ttl time.Duration
}

// NewSQLite builds a SQLite-backed auth store.
func NewSQLite(db *gorm.DB, cfg Config) (Store, error) {
	if db == nil {
		return nil, fmt.Errorf("sqlite store requires database handle")
	}
	return &sqliteStore{
		db:  db,
		ttl: cfg.TTL,
	}, nil
}

func (s *sqliteStore) Store(ctx context.Context, info model.ClientInfo) error {
	now := time.Now()
	if info.CreatedAt.IsZero() {
		info.CreatedAt = now
	}
	if info.ExpiresAt == nil && s.ttl > 0 {
		exp := info.CreatedAt.Add(s.ttl)
		info.ExpiresAt = &exp
	}
	meta, _ := json.Marshal(info.Metadata)

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("client_id = ?", info.ClientID).Delete(&storage.AuthClient{}).Error; err != nil {
			return err
		}
		record := &storage.AuthClient{
			ClientID:  info.ClientID,
			Username:  info.Username,
			Password:  info.Password,
			IP:        info.IP,
			DeviceID:  info.DeviceID,
			CreatedAt: info.CreatedAt,
			ExpiresAt: info.ExpiresAt,
			Metadata:  meta,
		}
		return tx.Create(record).Error
	})
}

func (s *sqliteStore) Validate(
	ctx context.Context,
	clientID string,
	username string,
	password string,
) (model.ClientInfo, bool, error) {
	authClient, err := s.fetch(ctx, clientID)
	if err != nil {
		if errorsIsNotFound(err) {
			return model.ClientInfo{}, false, nil
		}
		return model.ClientInfo{}, false, err
	}
	if authClient.Username != username || authClient.Password != password {
		return authClient, false, nil
	}
	if authClient.ExpiresAt != nil && time.Now().After(*authClient.ExpiresAt) {
		return model.ClientInfo{}, false, fmt.Errorf("expired credentials")
	}
	return authClient, true, nil
}

func (s *sqliteStore) Get(ctx context.Context, clientID string) (model.ClientInfo, error) {
	info, err := s.fetch(ctx, clientID)
	if err != nil {
		return model.ClientInfo{}, err
	}
	if info.ExpiresAt != nil && time.Now().After(*info.ExpiresAt) {
		return model.ClientInfo{}, fmt.Errorf("client expired: %s", clientID)
	}
	return info, nil
}

func (s *sqliteStore) Remove(ctx context.Context, clientID string) error {
	return s.db.WithContext(ctx).Where("client_id = ?", clientID).Delete(&storage.AuthClient{}).Error
}

func (s *sqliteStore) List(ctx context.Context) ([]string, error) {
	var clients []storage.AuthClient
	if err := s.db.WithContext(ctx).Select("client_id", "expires_at").Find(&clients).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	ids := make([]string, 0, len(clients))
	for _, c := range clients {
		if c.ExpiresAt == nil || now.Before(*c.ExpiresAt) {
			ids = append(ids, c.ClientID)
		}
	}
	return ids, nil
}

func (s *sqliteStore) CleanupExpired(ctx context.Context) error {
	if s.ttl <= 0 {
		return nil
	}
	return s.db.WithContext(ctx).
		Where("expires_at IS NOT NULL AND expires_at < ?", time.Now()).
		Delete(&storage.AuthClient{}).
		Error
}

func (s *sqliteStore) Stats(ctx context.Context) (map[string]any, error) {
	var total int64
	if err := s.db.WithContext(ctx).Model(&storage.AuthClient{}).Count(&total).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"type":  "sqlite",
		"total": total,
		"ttl":   int(s.ttl.Seconds()),
	}, nil
}

func (s *sqliteStore) Close(context.Context) error {
	return nil
}

func (s *sqliteStore) fetch(ctx context.Context, clientID string) (model.ClientInfo, error) {
	var client storage.AuthClient
	err := s.db.WithContext(ctx).Where("client_id = ?", clientID).First(&client).Error
	if errorsIsNotFound(err) {
		return model.ClientInfo{}, fmt.Errorf("client not found: %s", clientID)
	}
	if err != nil {
		return model.ClientInfo{}, err
	}
	info := model.ClientInfo{
		ClientID:  client.ClientID,
		Username:  client.Username,
		Password:  client.Password,
		IP:        client.IP,
		DeviceID:  client.DeviceID,
		CreatedAt: client.CreatedAt,
		ExpiresAt: client.ExpiresAt,
	}
	if len(client.Metadata) > 0 {
		var meta map[string]any
		if err := json.Unmarshal(client.Metadata, &meta); err == nil {
			info.Metadata = meta
		}
	}
	return info, nil
}

func errorsIsNotFound(err error) bool {
	return err != nil && errors.Is(err, gorm.ErrRecordNotFound)
}
