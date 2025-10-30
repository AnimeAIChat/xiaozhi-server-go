package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"xiaozhi-server-go/internal/domain/eventbus/repository"
	"xiaozhi-server-go/internal/platform/storage"
	"xiaozhi-server-go/internal/platform/errors"
)

// eventRepository 事件存储库实现
type eventRepository struct {
	db *gorm.DB
}

// NewEventRepository 创建事件存储库
func NewEventRepository(db *gorm.DB) repository.EventRepository {
	return &eventRepository{
		db: db,
	}
}

func (r *eventRepository) Store(ctx context.Context, event repository.Event) error {
	// 将事件数据序列化为JSON
	dataBytes, err := json.Marshal(event.Data)
	if err != nil {
		return errors.Wrap(errors.KindStorage, "event.store.marshal", "failed to marshal event data", err)
	}

	domainEvent := &storage.DomainEvent{
		EventType: event.EventType,
		SessionID: event.SessionID,
		UserID:    event.UserID,
		Data:      dataBytes,
		CreatedAt: event.CreatedAt,
	}

	if err := r.db.WithContext(ctx).Create(domainEvent).Error; err != nil {
		return errors.Wrap(errors.KindStorage, "event.store.create", "failed to store event", err)
	}

	return nil
}

func (r *eventRepository) FindBySessionID(ctx context.Context, sessionID string) ([]repository.Event, error) {
	var domainEvents []storage.DomainEvent
	if err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at ASC").
		Find(&domainEvents).Error; err != nil {
		return nil, errors.Wrap(errors.KindStorage, "event.find.session", "failed to find events by session ID", err)
	}

	return r.convertDomainEvents(domainEvents)
}

func (r *eventRepository) FindByEventType(ctx context.Context, eventType string, limit int) ([]repository.Event, error) {
	var domainEvents []storage.DomainEvent
	query := r.db.WithContext(ctx).
		Where("event_type = ?", eventType).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&domainEvents).Error; err != nil {
		return nil, errors.Wrap(errors.KindStorage, "event.find.type", "failed to find events by type", err)
	}

	return r.convertDomainEvents(domainEvents)
}

func (r *eventRepository) FindByTimeRange(ctx context.Context, startTime, endTime time.Time) ([]repository.Event, error) {
	var domainEvents []storage.DomainEvent
	if err := r.db.WithContext(ctx).
		Where("created_at BETWEEN ? AND ?", startTime, endTime).
		Order("created_at ASC").
		Find(&domainEvents).Error; err != nil {
		return nil, errors.Wrap(errors.KindStorage, "event.find.time", "failed to find events by time range", err)
	}

	return r.convertDomainEvents(domainEvents)
}

func (r *eventRepository) FindByUserID(ctx context.Context, userID string) ([]repository.Event, error) {
	var domainEvents []storage.DomainEvent
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at ASC").
		Find(&domainEvents).Error; err != nil {
		return nil, errors.Wrap(errors.KindStorage, "event.find.user", "failed to find events by user ID", err)
	}

	return r.convertDomainEvents(domainEvents)
}

func (r *eventRepository) DeleteOldEvents(ctx context.Context, beforeTime time.Time) error {
	if err := r.db.WithContext(ctx).
		Where("created_at < ?", beforeTime).
		Delete(&storage.DomainEvent{}).Error; err != nil {
		return errors.Wrap(errors.KindStorage, "event.delete.old", "failed to delete old events", err)
	}

	return nil
}

func (r *eventRepository) GetEventStats(ctx context.Context) (map[string]int64, error) {
	var stats []struct {
		EventType string
		Count     int64
	}

	if err := r.db.WithContext(ctx).
		Model(&storage.DomainEvent{}).
		Select("event_type, count(*) as count").
		Group("event_type").
		Scan(&stats).Error; err != nil {
		return nil, errors.Wrap(errors.KindStorage, "event.stats", "failed to get event stats", err)
	}

	result := make(map[string]int64)
	for _, stat := range stats {
		result[stat.EventType] = stat.Count
	}

	return result, nil
}

// convertDomainEvents 将数据库事件转换为领域事件
func (r *eventRepository) convertDomainEvents(domainEvents []storage.DomainEvent) ([]repository.Event, error) {
	events := make([]repository.Event, len(domainEvents))

	for i, de := range domainEvents {
		var data interface{}
		if len(de.Data) > 0 {
			if err := json.Unmarshal(de.Data, &data); err != nil {
				return nil, errors.Wrap(errors.KindStorage, "event.convert.unmarshal", "failed to unmarshal event data", err)
			}
		}

		events[i] = repository.Event{
			ID:        fmt.Sprintf("%d", de.ID), // 转换为字符串ID
			EventType: de.EventType,
			SessionID: de.SessionID,
			UserID:    de.UserID,
			Data:      data,
			CreatedAt: de.CreatedAt,
		}
	}

	return events, nil
}