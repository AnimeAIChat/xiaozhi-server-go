package repository

import (
	"context"
	"time"

	"xiaozhi-server-go/internal/domain/eventbus"
)

// EventRepository 领域事件数据访问接口
type EventRepository interface {
	// Store 存储领域事件
	Store(ctx context.Context, event Event) error

	// FindBySessionID 根据会话ID查找事件
	FindBySessionID(ctx context.Context, sessionID string) ([]Event, error)

	// FindByEventType 根据事件类型查找事件
	FindByEventType(ctx context.Context, eventType string, limit int) ([]Event, error)

	// FindByTimeRange 根据时间范围查找事件
	FindByTimeRange(ctx context.Context, startTime, endTime time.Time) ([]Event, error)

	// FindByUserID 根据用户ID查找事件
	FindByUserID(ctx context.Context, userID string) ([]Event, error)

	// DeleteOldEvents 删除指定时间之前的旧事件
	DeleteOldEvents(ctx context.Context, beforeTime time.Time) error

	// GetEventStats 获取事件统计信息
	GetEventStats(ctx context.Context) (map[string]int64, error)
}

// Event 领域事件
type Event struct {
	ID        string
	EventType string
	SessionID string
	UserID    string
	Data      interface{}
	CreatedAt time.Time
}