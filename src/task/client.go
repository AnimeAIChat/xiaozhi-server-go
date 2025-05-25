package task

import (
	"fmt"
	"sync"
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
	quota := &ResourceQuota{
		MaxImageTasks:     50,  // Default daily limit
		MaxVideoTasks:     20,  // Default daily limit
		MaxScheduledTasks: 100, // Default limit
		UsedQuota:         make(map[TaskType]int),
		MaxConcurrent:     make(map[TaskType]int),
		CurrentRunning:    make(map[TaskType]int),
	}

	// 设置默认并发限制
	quota.MaxConcurrent[TaskTypeImageGen] = 5
	quota.MaxConcurrent[TaskTypeVideoGen] = 2
	quota.MaxConcurrent[TaskTypeScheduled] = 10

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
		rq.MaxImageTasks = 50
		rq.MaxVideoTasks = 20
		rq.MaxScheduledTasks = 100
		rq.MaxConcurrent[TaskTypeImageGen] = 5
		rq.MaxConcurrent[TaskTypeVideoGen] = 2
		rq.MaxConcurrent[TaskTypeScheduled] = 10
	case UserLevelPremium:
		rq.MaxImageTasks = 200
		rq.MaxVideoTasks = 50
		rq.MaxScheduledTasks = 300
		rq.MaxConcurrent[TaskTypeImageGen] = 10
		rq.MaxConcurrent[TaskTypeVideoGen] = 5
		rq.MaxConcurrent[TaskTypeScheduled] = 20
	case UserLevelBusiness:
		rq.MaxImageTasks = 500
		rq.MaxVideoTasks = 200
		rq.MaxScheduledTasks = 1000
		rq.MaxConcurrent[TaskTypeImageGen] = 30
		rq.MaxConcurrent[TaskTypeVideoGen] = 15
		rq.MaxConcurrent[TaskTypeScheduled] = 50
	}
}

// CanAcceptTask checks if a task can be accepted based on quotas
func (rq *ResourceQuota) CanAcceptTask(taskType TaskType) bool {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	// 检查总配额
	var quotaAvailable bool
	switch taskType {
	case TaskTypeImageGen:
		quotaAvailable = rq.UsedQuota[TaskTypeImageGen] < rq.MaxImageTasks
	case TaskTypeVideoGen:
		quotaAvailable = rq.UsedQuota[TaskTypeVideoGen] < rq.MaxVideoTasks
	case TaskTypeScheduled:
		quotaAvailable = rq.UsedQuota[TaskTypeScheduled] < rq.MaxScheduledTasks
	default:
		return false
	}

	// 检查并发限制
	concurrencyAvailable := rq.CurrentRunning[taskType] < rq.MaxConcurrent[taskType]

	return quotaAvailable && concurrencyAvailable
}

// StartTask marks a task as started and increments the running count
func (rq *ResourceQuota) StartTask(taskType TaskType) error {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	// 检查是否超出并发限制
	if rq.CurrentRunning[taskType] >= rq.MaxConcurrent[taskType] {
		return fmt.Errorf("maximum concurrent %v tasks reached", taskType)
	}

	rq.CurrentRunning[taskType]++
	return nil
}

// CompleteTask marks a task as completed and decrements the running count
func (rq *ResourceQuota) CompleteTask(taskType TaskType) {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	if rq.CurrentRunning[taskType] > 0 {
		rq.CurrentRunning[taskType]--
	}
}

// ResetConcurrencyCounts resets all current running counts
func (rq *ResourceQuota) ResetConcurrencyCounts() {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	for taskType := range rq.CurrentRunning {
		rq.CurrentRunning[taskType] = 0
	}
}

// IncrementQuota increments the used quota for a task type
func (rq *ResourceQuota) IncrementQuota(taskType TaskType) error {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	var maxQuota int
	var quotaExceededMsg string

	switch taskType {
	case TaskTypeImageGen:
		maxQuota = rq.MaxImageTasks
		quotaExceededMsg = "image generation quota exceeded"
	case TaskTypeVideoGen:
		maxQuota = rq.MaxVideoTasks
		quotaExceededMsg = "video generation quota exceeded"
	case TaskTypeScheduled:
		maxQuota = rq.MaxScheduledTasks
		quotaExceededMsg = "scheduled task quota exceeded"
	default:
		return fmt.Errorf("unknown task type: %v", taskType)
	}

	if rq.UsedQuota[taskType] >= maxQuota {
		return fmt.Errorf(quotaExceededMsg)
	}

	rq.UsedQuota[taskType]++
	return nil
}

// DecrementQuota decrements the used quota for a task type
func (rq *ResourceQuota) DecrementQuota(taskType TaskType) {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	if rq.UsedQuota[taskType] > 0 {
		rq.UsedQuota[taskType]--
	}
}

// ResetQuota resets quotas for a task type
func (rq *ResourceQuota) ResetQuota(taskType TaskType) {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	rq.UsedQuota[taskType] = 0
}

// ResetAllQuotas resets all quotas
func (rq *ResourceQuota) ResetAllQuotas() {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	for taskType := range rq.UsedQuota {
		rq.UsedQuota[taskType] = 0
	}
}
