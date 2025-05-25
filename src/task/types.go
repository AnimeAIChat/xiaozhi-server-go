package task

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TaskType represents different types of async tasks
type TaskType string

const (
	TaskTypeImageGen  TaskType = "image_gen"
	TaskTypeVideoGen  TaskType = "video_gen"
	TaskTypeScheduled TaskType = "scheduled"
)

// TaskStatus represents the current status of a task
type TaskStatus string

const (
	TaskStatusPending  TaskStatus = "pending"
	TaskStatusRunning  TaskStatus = "running"
	TaskStatusComplete TaskStatus = "complete"
	TaskStatusFailed   TaskStatus = "failed"
)

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
}

func NewTask(taskType TaskType, params interface{}, callback TaskCallback) (task *Task, id string) {
	id = uuid.New().String()
	return &Task{
		ID:        id,
		Type:      taskType,
		Status:    TaskStatusPending,
		Params:    params,
		Callback:  callback,
		CreatedAt: time.Now(),
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

	t.Status = TaskStatusRunning
	t.UpdatedAt = time.Now()

	// Execute task based on type
	switch t.Type {
	case TaskTypeImageGen:
		t.executeImageGen()
	case TaskTypeVideoGen:
		t.executeVideoGen()
	case TaskTypeScheduled:
		t.executeScheduled()
	default:
		t.Error = fmt.Errorf("unknown task type: %v", t.Type)
		t.Status = TaskStatusFailed
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

func (t *Task) executeImageGen() {
	t.Result = "image_url_here" // Placeholder for image generation logic
}

func (t *Task) executeVideoGen() {
	// TODO: Implement video generation logic
	t.Result = "video_url_here"
}

func (t *Task) executeScheduled() {
	// Execute scheduled task based on params
	if params, ok := t.Params.(map[string]interface{}); ok {
		if action, ok := params["action"].(string); ok {
			switch action {
			case "play_music":
				// Handle music playback
				t.Result = "Music played successfully"
			default:
				t.Error = fmt.Errorf("unknown scheduled action: %v", action)
			}
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
	MaxImageTasks     int
	MaxVideoTasks     int
	MaxScheduledTasks int
	UsedQuota         map[TaskType]int
	MaxConcurrent     map[TaskType]int // 每种任务类型的最大并发数
	CurrentRunning    map[TaskType]int // 每种任务类型当前正在运行的数量
	UserLevel         UserLevel        // 新增用户级别字段
	mu                sync.RWMutex
}

// ClientContext holds client-specific settings and state
type ClientContext struct {
	ID                 string
	MaxConcurrentTasks int
	TaskQueue          chan *Task
	ActiveTasks        map[string]*Task
	ResourceQuota      *ResourceQuota
	mu                 sync.RWMutex
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
	MaxWorkers          int
	MaxTasksPerClient   int
	MaxImageTasksPerDay int
	MaxVideoTasksPerDay int
	MaxScheduledTasks   int
}
