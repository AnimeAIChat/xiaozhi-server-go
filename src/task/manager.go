package task

import (
	"fmt"
	"sync"
	"time"
)

// TaskManager manages async tasks and their execution
type TaskManager struct {
	workerPool     *WorkerPool
	scheduledTasks *ScheduledTasks
	clientManager  *ClientManager
}

// NewTaskManager creates a new TaskManager instance
func NewTaskManager(config ResourceConfig) *TaskManager {
	tm := &TaskManager{
		clientManager: NewClientManager(),
	}

	tm.workerPool = NewWorkerPool(config, nil, tm.clientManager)
	tm.scheduledTasks = NewScheduledTasks(tm.workerPool)
	tm.workerPool.scheduler = tm.scheduledTasks

	return tm
}

// Start starts the task manager and its components
func (tm *TaskManager) Start() {
	tm.workerPool.Start()
	tm.scheduledTasks.Start()
}

// Stop stops the task manager and its components
func (tm *TaskManager) Stop() {
	tm.workerPool.Stop()
	tm.scheduledTasks.Stop()
}

// SubmitTask submits a task for execution
func (tm *TaskManager) SubmitTask(clientID string, task *Task) error {
	// 检查任务类型是否已注册
	if _, exists := GetTaskExecutor(task.Type); !exists {
		return fmt.Errorf("task type %v is not registered", task.Type)
	}

	if task.ScheduledTime != nil {
		return tm.scheduleTask(clientID, task)
	}
	return tm.submitImmediateTask(clientID, task)
}

// submitImmediateTask submits a task for immediate execution
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

	// 提交到工作池，失败时回滚
	if err := tm.workerPool.Submit(task); err != nil {
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

// ScheduledTasks manages scheduled tasks
type ScheduledTasks struct {
	tasks      map[string]*Task
	ticker     *time.Ticker
	stopChan   chan struct{}
	workerPool *WorkerPool
	mu         sync.RWMutex
}

// NewScheduledTasks creates a new ScheduledTasks instance
func NewScheduledTasks(workerPool *WorkerPool) *ScheduledTasks {
	return &ScheduledTasks{
		tasks:      make(map[string]*Task),
		ticker:     time.NewTicker(time.Second),
		stopChan:   make(chan struct{}),
		workerPool: workerPool,
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

// processScheduledTasks checks and executes due tasks
func (st *ScheduledTasks) processScheduledTasks() {
	now := time.Now()
	st.mu.Lock()
	defer st.mu.Unlock()

	// 每日重置检查（复用现有定时器）
	if st.workerPool.clientManager != nil {
		st.workerPool.clientManager.checkDailyReset()
	}

	for id, task := range st.tasks {
		if task.ScheduledTime.Before(now) || task.ScheduledTime.Equal(now) {
			// 使用工作者池执行，而非直接go
			if err := st.workerPool.Submit(task); err != nil {
				// 提交失败的降级处理
				go func(t *Task) {
					defer func() {
						if r := recover(); r != nil {
							fmt.Printf("Scheduled task panic: %v\n", r)
						}
					}()
					t.Execute()
				}(task)
			}
			delete(st.tasks, id)
		}
	}
}
