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
	mu             sync.RWMutex
}

// NewTaskManager creates a new TaskManager instance
func NewTaskManager(config ResourceConfig) *TaskManager {
	tm := &TaskManager{
		scheduledTasks: NewScheduledTasks(),
		clientManager:  NewClientManager(),
	}
	tm.workerPool = NewWorkerPool(config, tm.scheduledTasks)
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

	// Check resource quotas
	if !ctx.ResourceQuota.CanAcceptTask(task.Type) {
		return fmt.Errorf("resource quota exceeded for task type: %v", task.Type)
	}

	// Submit to worker pool
	return tm.workerPool.Submit(task)
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

	if !ctx.ResourceQuota.CanAcceptTask(TaskTypeScheduled) {
		return fmt.Errorf("scheduled task quota exceeded")
	}

	tm.scheduledTasks.AddTask(task)
	return nil
}

// ScheduledTasks manages scheduled tasks
type ScheduledTasks struct {
	tasks    map[string]*Task
	ticker   *time.Ticker
	stopChan chan struct{}
	mu       sync.RWMutex
}

// NewScheduledTasks creates a new ScheduledTasks instance
func NewScheduledTasks() *ScheduledTasks {
	return &ScheduledTasks{
		tasks:    make(map[string]*Task),
		ticker:   time.NewTicker(time.Second),
		stopChan: make(chan struct{}),
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

	for id, task := range st.tasks {
		if task.ScheduledTime.Before(now) || task.ScheduledTime.Equal(now) {
			go task.Execute()
			delete(st.tasks, id)
		}
	}
}
