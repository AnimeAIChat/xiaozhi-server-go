package task

import (
	"fmt"
	"sync"
	"time"
)

// ClientManager manages client contexts and resources
type ClientManager struct {
	clients map[string]*ClientContext
	mu      sync.RWMutex
}

// NewClientManager creates a new client manager
func NewClientManager() *ClientManager {
	return &ClientManager{
		clients: make(map[string]*ClientContext),
	}
}

// GetClientContext gets or creates a client context
func (cm *ClientManager) GetClientContext(clientID string) (*ClientContext, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if ctx, exists := cm.clients[clientID]; exists {
		return ctx, nil
	}

	// Create new client context
	ctx := &ClientContext{
		ID:                 clientID,
		MaxConcurrentTasks: 10, // Default value, should be configurable
		TaskQueue:          make(chan *Task, 100),
		ActiveTasks:        make(map[string]*Task),
		ResourceQuota:      NewResourceQuota(),
	}

	cm.clients[clientID] = ctx
	return ctx, nil
}

func (cm *ClientManager) checkDailyReset() {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for _, ctx := range cm.clients {
		ctx.ResourceQuota.CheckAndResetDailyQuota()
	}
}

// RemoveClient removes a client context
func (cm *ClientManager) RemoveClient(clientID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if ctx, exists := cm.clients[clientID]; exists {
		close(ctx.TaskQueue)
		delete(cm.clients, clientID)
	}
}

// NewResourceQuota creates a new resource quota instance
// TODO: Add configuration for max tasks
//
// TODO	不同级别的用户可以设置不同的配额
func NewResourceQuota() *ResourceQuota {
	now := time.Now()
	quota := &ResourceQuota{
		MaxTotalTasks:      100, // Default daily total limit
		MaxConcurrentTasks: 10,  // Default concurrent limit
		TotalUsedQuota:     0,
		TotalRunningTasks:  0,
		UserLevel:          UserLevelBasic,
		LastResetDate: time.Date(
			now.Year(),
			now.Month(),
			now.Day(),
			0,
			0,
			0,
			0,
			now.Location(),
		),
	}

	return quota
}

// 设置用户配额的方法
func (rq *ResourceQuota) SetUserLevel(level UserLevel) {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	rq.UserLevel = level

	// 根据用户级别设置不同的配额
	switch level {
	case UserLevelBasic:
		rq.MaxTotalTasks = 100
		rq.MaxConcurrentTasks = 5
	case UserLevelPremium:
		rq.MaxTotalTasks = 500
		rq.MaxConcurrentTasks = 15
	case UserLevelBusiness:
		rq.MaxTotalTasks = 2000
		rq.MaxConcurrentTasks = 50
	}
}

func (rq *ResourceQuota) CheckAndResetDailyQuota() {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// 如果距离上次重置已经过了一天
	if rq.LastResetDate.Before(today) {
		rq.TotalUsedQuota = 0
		rq.LastResetDate = today
		fmt.Printf("每日配额已重置，客户端时间: %s\n", today.Format("2006-01-02"))
	}
}

func (rq *ResourceQuota) TryIncrementQuota() error {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	// 原子检查和增加
	if rq.TotalUsedQuota >= rq.MaxTotalTasks {
		return fmt.Errorf("daily task quota exceeded")
	}
	if rq.TotalRunningTasks >= rq.MaxConcurrentTasks {
		return fmt.Errorf("concurrent task limit exceeded")
	}

	rq.TotalUsedQuota++
	rq.TotalRunningTasks++
	return nil
}

// CompleteTask marks a task as completed and decrements the running count
func (rq *ResourceQuota) CompleteTask(taskType TaskType) {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	rq.TotalRunningTasks--
}

// DecrementQuota decrements the used quota for a task type
func (rq *ResourceQuota) DecrementQuota(taskType TaskType) {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	if rq.TotalUsedQuota > 0 {
		rq.TotalUsedQuota--
	}
}

// ResetQuota resets quotas for a task type
func (rq *ResourceQuota) ResetQuota(taskType TaskType) {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	rq.TotalUsedQuota = 0
}
