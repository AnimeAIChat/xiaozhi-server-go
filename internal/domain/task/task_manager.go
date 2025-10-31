package task

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"xiaozhi-server-go/internal/util/work"
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

// ClientManager manages client contexts and resource quotas
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

// SetUserLevel sets the user level for quota management
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

// TaskJob represents a task job for the new work pool
type TaskJob struct {
	Task     *Task
	ClientID string
}

// NewTaskManager creates a new TaskManager instance using the new work components
func NewTaskManager(config ResourceConfig) *TaskManager {
	// Create work pool handler
	var taskHandler work.JobHandler = func(job work.Job) error {
		taskJob, ok := job.(TaskJob)
		if !ok {
			return fmt.Errorf("invalid job type: expected TaskJob")
		}
		return executeTask(taskJob.Task)
	}

	// Create work pool with reasonable defaults
	numWorkers := config.MaxWorkers
	if numWorkers <= 0 {
		numWorkers = 8
	}
	queueSize := config.MaxTasksPerClient * numWorkers
	if queueSize <= 0 {
		queueSize = 160 // default queue size
	}

	workPool := work.NewWorkPool(numWorkers, queueSize, taskHandler)

	tm := &TaskManager{
		workPool:      workPool,
		scheduledTasks: NewScheduledTasks(nil), // We'll handle scheduling differently
		clientManager: NewClientManager(),
		config:        config,
	}

	// Set up scheduled tasks to use the work pool
	tm.scheduledTasks = NewScheduledTasks(tm)

	return tm
}

// executeTask executes a single task
func executeTask(task *Task) error {
	defer func() {
		if r := recover(); r != nil {
			task.Status = "failed"
			task.Error = fmt.Errorf("task panicked: %v", r)
			if task.Callback != nil {
				task.Callback.OnError(task.Error)
			}
		}
	}()

	select {
	case <-task.Context.Done():
		fmt.Printf("任务 %s 因连接断开而取消\n", task.ID)
		return nil
	default:
	}

		task.Status = "running"
	task.UpdatedAt = time.Now()

	executor, exists := GetTaskExecutor(task.Type)
	if !exists {
		task.Error = fmt.Errorf("no executor registered for task type: %v", task.Type)
		task.Status = "failed"
		if task.Callback != nil {
			task.Callback.OnError(task.Error)
		}
		return task.Error
	}

	// Execute the task using the registered executor
	task.Error = executor(task)

	// Call appropriate callback
	if task.Error != nil {
		task.Status = "failed"
		if task.Callback != nil {
			task.Callback.OnError(task.Error)
		}
	} else {
		task.Status = "complete"
		if task.Callback != nil {
			task.Callback.OnComplete(task.Result)
		}
	}

	return task.Error
}

// SubmitTask submits a task for execution using the new work pool
func (tm *TaskManager) SubmitTask(clientID string, task *Task) error {
	// 检查任务类型是否已注册
	_, exists := GetTaskExecutor(task.Type)
	if !exists {
		return fmt.Errorf("task type %v is not registered", task.Type)
	}

	if task.ScheduledTime != nil {
		return tm.scheduleTask(clientID, task)
	}
	return tm.submitImmediateTask(clientID, task)
}

// submitImmediateTask submits a task for immediate execution using work pool
func (tm *TaskManager) submitImmediateTask(clientID string, task *Task) error {
	// Get or create client context
	ctx, err := tm.clientManager.GetClientContext(clientID)
	if err != nil {
		return fmt.Errorf("failed to get client context: %v", err)
	}

	// 原子检查和增加配额
	if err := ctx.ResourceQuota.TryIncrementQuota(); err != nil {
		return err
	}

	task.ClinetID = clientID

	// Create job for work pool
	job := TaskJob{
		Task:     task,
		ClientID: clientID,
	}

	// Submit to work pool
	if err := tm.workPool.Submit(job); err != nil {
		ctx.ResourceQuota.DecrementQuota(task.Type) // 减少总配额
		ctx.ResourceQuota.CompleteTask(task.Type)   // 减少并发计数
		return err
	}

	return nil
}

// scheduleTask schedules a task for future execution
func (tm *TaskManager) scheduleTask(clientID string, task *Task) error {
	if task.ScheduledTime == nil {
		return fmt.Errorf("scheduled time is required for scheduled tasks")
	}

	ctx, err := tm.clientManager.GetClientContext(clientID)
	if err != nil {
		return fmt.Errorf("failed to get client context: %v", err)
	}

	// 检查是否可以接受任务（使用总配额检查）
	if err := ctx.ResourceQuota.TryIncrementQuota(); err != nil {
		return err
	}

	tm.scheduledTasks.AddTask(task)
	return nil
}

// Start starts the task manager and its components
func (tm *TaskManager) Start() {
	tm.scheduledTasks.Start()
}

// Stop stops the task manager and its components
func (tm *TaskManager) Stop() {
	tm.workPool.Stop()
	tm.scheduledTasks.Stop()
}

// TaskManager struct with new work pool
type TaskManager struct {
	workPool       *work.Pool
	scheduledTasks *ScheduledTasks
	clientManager  *ClientManager
	config         ResourceConfig
}

// ScheduledTasks manages scheduled tasks with work pool integration
type ScheduledTasks struct {
	tasks      map[string]*Task
	ticker     *time.Ticker
	stopChan   chan struct{}
	taskMgr    *TaskManager // Reference to task manager for work pool access
	mu         sync.RWMutex
}

// NewScheduledTasks creates a new ScheduledTasks instance
func NewScheduledTasks(taskMgr *TaskManager) *ScheduledTasks {
	return &ScheduledTasks{
		tasks:    make(map[string]*Task),
		ticker:   time.NewTicker(time.Second),
		stopChan: make(chan struct{}),
		taskMgr:  taskMgr,
	}
}

// Start starts the scheduled tasks processor
func (st *ScheduledTasks) Start() {
	go st.run()
}

// Stop stops the scheduled tasks processor
func (st *ScheduledTasks) Stop() {
	st.ticker.Stop()
	close(st.stopChan)
}

// AddTask adds a new scheduled task
func (st *ScheduledTasks) AddTask(task *Task) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.tasks[task.ID] = task
}

// run processes scheduled tasks
func (st *ScheduledTasks) run() {
	for {
		select {
		case <-st.stopChan:
			return
		case <-st.ticker.C:
			st.processScheduledTasks()
		}
	}
}

// processScheduledTasks checks and executes due tasks using work pool
func (st *ScheduledTasks) processScheduledTasks() {
	now := time.Now()
	st.mu.Lock()
	defer st.mu.Unlock()

	// 每日重置检查（暂时跳过，因为方法未导出）
	// if st.taskMgr.clientManager != nil {
	//     st.taskMgr.clientManager.checkDailyReset()
	// }

	for id, task := range st.tasks {
		if task.ScheduledTime.Before(now) || task.ScheduledTime.Equal(now) {
			// Use work pool to execute the task
			job := TaskJob{
				Task:     task,
				ClientID: task.ClinetID,
			}

			if err := st.taskMgr.workPool.Submit(job); err != nil {
				// 提交失败的降级处理
				go func(t *Task) {
					defer func() {
						if r := recover(); r != nil {
							fmt.Printf("Scheduled task panic: %v\n", r)
						}
					}()
					executeTask(t)
				}(task)
			}
			delete(st.tasks, id)
		}
	}
}

// CallBack represents a task completion callback
type CallBack struct {
	taskCallback func(result interface{})
}

// NewCallBack creates a new callback instance
func NewCallBack(callback func(result interface{})) *CallBack {
	return &CallBack{
		taskCallback: callback,
	}
}

// OnComplete handles task completion
func (cb *CallBack) OnComplete(result interface{}) {
	if cb.taskCallback != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("Callback panic recovered: %v\n", r)
				}
			}()
			cb.taskCallback(result)
		}()
	}
}

// OnError handles task errors
func (cb *CallBack) OnError(err error) {
	if cb.taskCallback != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("Error callback panic recovered: %v\n", r)
				}
			}()
			result := map[string]interface{}{
				"error":  err.Error(),
				"status": "failed",
			}
			cb.taskCallback(result)
		}()
	}
}