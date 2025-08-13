package task

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TaskType represents different types of async tasks
type TaskType string

// TaskStatus represents the current status of a task
type TaskStatus string

// TaskExecutor defines the function signature for task execution
type TaskExecutor func(t *Task) error

const (
	TaskStatusPending  TaskStatus = "pending"
	TaskStatusRunning  TaskStatus = "running"
	TaskStatusComplete TaskStatus = "complete"
	TaskStatusFailed   TaskStatus = "failed"
)

// TaskRegistry manages task type to executor mappings
type TaskRegistry struct {
	executors map[TaskType]TaskExecutor
	mu        sync.RWMutex
}

// Global task registry instance
var taskRegistry = &TaskRegistry{
	executors: make(map[TaskType]TaskExecutor),
}

// RegisterTaskExecutor registers a task executor for a specific task type
func RegisterTaskExecutor(taskType TaskType, executor TaskExecutor) {
	taskRegistry.mu.Lock()
	defer taskRegistry.mu.Unlock()
	taskRegistry.executors[taskType] = executor
	// fmt.Printf("注册任务类型: %s\n", taskType)
}

// GetTaskExecutor retrieves the executor for a specific task type
func GetTaskExecutor(taskType TaskType) (TaskExecutor, bool) {
	taskRegistry.mu.RLock()
	defer taskRegistry.mu.RUnlock()
	executor, exists := taskRegistry.executors[taskType]
	return executor, exists
}

// GetRegisteredTaskTypes returns all registered task types
func GetRegisteredTaskTypes() []TaskType {
	taskRegistry.mu.RLock()
	defer taskRegistry.mu.RUnlock()
	types := make([]TaskType, 0, len(taskRegistry.executors))
	for taskType := range taskRegistry.executors {
		types = append(types, taskType)
	}
	return types
}

// Task represents an async task with its properties and callback
type Task struct {
	ID            string
	Type          TaskType
	Status        TaskStatus
	Params        interface{}
	Result        interface{}
	Error         error
	ScheduledTime *time.Time
	Callback      TaskCallback
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ClinetID      string
	Context       context.Context
}

func NewTask(ctx context.Context, taskType TaskType, params interface{}) (task *Task, id string) {
	id = uuid.New().String()
	return &Task{
		ID:        id,
		Type:      taskType,
		Status:    TaskStatusPending,
		Params:    params,
		CreatedAt: time.Now(),
		Context:   ctx,
	}, id
}

// Execute executes the task and calls appropriate callbacks
func (t *Task) Execute() {
	defer func() {
		if r := recover(); r != nil {
			t.Status = TaskStatusFailed
			t.Error = fmt.Errorf("task panicked: %v", r)
			if t.Callback != nil {
				t.Callback.OnError(t.Error)
			}
		}
	}()

	select {
	case <-t.Context.Done():
		fmt.Printf("任务 %s 因连接断开而取消\n", t.ID)
		return
	default:
	}

	t.Status = TaskStatusRunning
	t.UpdatedAt = time.Now()

	executor, exists := GetTaskExecutor(t.Type)
	if !exists {
		t.Error = fmt.Errorf("no executor registered for task type: %v", t.Type)
		t.Status = TaskStatusFailed
	} else {
		// Execute the task using the registered executor
		t.Error = executor(t)
	}

	// Call appropriate callback
	if t.Error != nil {
		t.Status = TaskStatusFailed
		if t.Callback != nil {
			t.Callback.OnError(t.Error)
		}
	} else {
		t.Status = TaskStatusComplete
		if t.Callback != nil {
			t.Callback.OnComplete(t.Result)
		}
	}
}

// TaskCallback defines the interface for task completion handling
type TaskCallback interface {
	OnComplete(result interface{})
	OnError(err error)
}

type UserLevel string

const (
	UserLevelBasic    UserLevel = "basic"
	UserLevelPremium  UserLevel = "premium"
	UserLevelBusiness UserLevel = "business"
)

// ResourceQuota manages resource limits for tasks
type ResourceQuota struct {
	MaxTotalTasks      int       // 总任务配额限制
	MaxConcurrentTasks int       // 总并发任务限制
	TotalUsedQuota     int       // 总已使用配额
	TotalRunningTasks  int       // 总运行中任务数
	UserLevel          UserLevel // 新增用户级别字段
	LastResetDate      time.Time
	mu                 sync.RWMutex
}

// ClientContext holds client-specific settings and state
type ClientContext struct {
	ID                 string
	MaxConcurrentTasks int
	TaskQueue          chan *Task
	ActiveTasks        map[string]*Task
	ResourceQuota      *ResourceQuota
}

// WorkerStatus represents the current status of a worker
type WorkerStatus string

const (
	WorkerStatusIdle    WorkerStatus = "idle"
	WorkerStatusBusy    WorkerStatus = "busy"
	WorkerStatusStopped WorkerStatus = "stopped"
)

// ResourceConfig defines resource limits for task execution
type ResourceConfig struct {
	MaxWorkers        int
	MaxTasksPerClient int
}
