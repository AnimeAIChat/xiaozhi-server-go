package task

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// WorkerPool manages a pool of workers for executing tasks
type WorkerPool struct {
	config        ResourceConfig
	workers       []*Worker
	taskQueue     chan *Task
	scheduler     *ScheduledTasks
	stopChan      chan struct{}
	idleWorkers   chan *Worker
	clientManager *ClientManager
	mu            sync.RWMutex
}

// Worker represents a task execution worker
type Worker struct {
	id       string
	status   WorkerStatus
	taskChan chan *Task
	stopChan chan struct{}
	pool     *WorkerPool
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(
	config ResourceConfig,
	scheduler *ScheduledTasks,
	clientManager *ClientManager,
) *WorkerPool {
	wp := &WorkerPool{
		config:        config,
		taskQueue:     make(chan *Task, config.MaxWorkers*2),
		scheduler:     scheduler,
		stopChan:      make(chan struct{}),
		idleWorkers:   make(chan *Worker, config.MaxWorkers),
		clientManager: clientManager,
	}

	// Initialize worker types
	wp.initWorkers()
	return wp
}

// initWorkerTypes initializes worker pools for different task types
func (wp *WorkerPool) initWorkers() {
	wp.workers = make([]*Worker, wp.config.MaxWorkers)
	for i := 0; i < wp.config.MaxWorkers; i++ {
		worker := newWorker(fmt.Sprintf("worker-%d", i), wp)
		wp.workers[i] = worker
		// 初始化时所有工作者都是空闲的
		wp.idleWorkers <- worker
	}
}

// Start starts the worker pool
func (wp *WorkerPool) Start() {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	// Start all workers
	for _, worker := range wp.workers {
		go worker.start()
	}

	// Start task distribution
	go wp.distributeItems()
}

// Stop stops the worker pool
func (wp *WorkerPool) Stop() {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	close(wp.stopChan)
	for _, worker := range wp.workers {
		worker.stop()
	}
}

// Submit submits a task to the worker pool
func (wp *WorkerPool) Submit(task *Task) error {
	select {
	case wp.taskQueue <- task:
		return nil
	default:
		return fmt.Errorf("task queue is full")
	}
}

// distributeItems distributes tasks to appropriate workers
func (wp *WorkerPool) distributeItems() {
	for {
		select {
		case <-wp.stopChan:
			return
		case task := <-wp.taskQueue:
			wp.assignTask(task)
		}
	}
}

// 新增一个安全地重新排队的方法
func (wp *WorkerPool) requeueTask(task *Task) {
	select {
	case wp.taskQueue <- task:
		// 成功加入队列
	default:
		// 队列已满，处理这种情况
		// 可以记录日志，或尝试其他策略
		if task.Callback != nil {
			task.Error = fmt.Errorf("task queue is full, cannot process task")
			task.Callback.OnError(task.Error)
		}
	}
}

// assignTask assigns a task to an available worker
func (wp *WorkerPool) assignTask(task *Task) {
	// 检查是否有注册的执行器
	if _, exists := GetTaskExecutor(task.Type); !exists {
		task.Error = fmt.Errorf("no executor registered for task type: %v", task.Type)
		task.Status = TaskStatusFailed
		if task.Callback != nil {
			task.Callback.OnError(task.Error)
		}
		return
	}

	select {
	case worker := <-wp.idleWorkers:
		worker.assignTask(task)
	case <-time.After(10 * time.Second): // 10秒超时
		// 超时处理：直接失败，不重排队
		task.Status = TaskStatusFailed
		task.Error = fmt.Errorf("no available workers within timeout")
		if task.ClinetID != "" && wp.clientManager != nil {
			if ctx, err := wp.clientManager.GetClientContext(task.ClinetID); err == nil {
				ctx.ResourceQuota.DecrementQuota(task.Type)
				ctx.ResourceQuota.CompleteTask(task.Type)
			}
		}
		if task.Callback != nil {
			task.Callback.OnError(task.Error)
		}
	}
}

// workerFinished 当工作者完成任务时调用
func (wp *WorkerPool) workerFinished(worker *Worker) {
	select {
	case wp.idleWorkers <- worker:
		// 工作者重新加入空闲队列
	default:
		// 这种情况不应该发生，但为了安全起见
		fmt.Printf("Warning: Failed to return worker %s to idle pool\n", worker.id)
	}
}

// newWorker creates a new worker
func newWorker(id string, pool *WorkerPool) *Worker {
	return &Worker{
		id:       id,
		status:   WorkerStatusIdle,
		taskChan: make(chan *Task, 1),
		stopChan: make(chan struct{}),
		pool:     pool,
	}
}

// start starts the worker
func (w *Worker) start() {
	for {
		select {
		case <-w.stopChan:
			return
		case task := <-w.taskChan:
			w.executeTask(task)
		}
	}
}

// executeTask executes a task
func (w *Worker) executeTask(task *Task) {
	w.status = WorkerStatusBusy

	defer func() {
		w.status = WorkerStatusIdle
		w.pool.workerFinished(w)
		// 任务完成，减少并发计数
		if task.ClinetID != "" && w.pool.clientManager != nil {
			if ctx, err := w.pool.clientManager.GetClientContext(task.ClinetID); err == nil {
				ctx.ResourceQuota.CompleteTask(task.Type)
			}
		}
	}()
	// 创建带取消的context
	ctx, cancel := context.WithTimeout(task.Context, 5*time.Minute)
	defer cancel()

	// 在新的context中执行任务
	task.Context = ctx

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				task.Status = TaskStatusFailed
				task.Error = fmt.Errorf("task panicked: %v", r)
			}
		}()

		// 检查context是否已取消
		select {
		case <-ctx.Done():
			task.Status = TaskStatusFailed
			task.Error = ctx.Err()
			return
		default:
		}

		task.Execute()
	}()

	select {
	case <-done:
		// 任务正常完成
	case <-ctx.Done():
		// 超时或取消
		task.Status = TaskStatusFailed
		task.Error = ctx.Err()
		if task.Callback != nil {
			task.Callback.OnError(task.Error)
		}
	}
}

// stop stops the worker
func (w *Worker) stop() {
	w.status = WorkerStatusStopped
	close(w.stopChan)
}

// assignTask assigns a task to the worker
func (w *Worker) assignTask(task *Task) {
	select {
	case w.taskChan <- task:
		// 任务成功分配
	default:
		// 这种情况不应该发生，因为 taskChan 有缓冲
		fmt.Printf("Warning: Failed to assign task to worker %s\n", w.id)
	}
}
