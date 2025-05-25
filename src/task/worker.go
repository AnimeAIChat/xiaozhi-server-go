package task

import (
	"fmt"
	"sync"
	"time"
)

// WorkerPool manages a pool of workers for executing tasks
type WorkerPool struct {
	config      ResourceConfig
	workers     []*Worker
	taskQueue   chan *Task
	scheduler   *ScheduledTasks
	stopChan    chan struct{}
	workerTypes map[TaskType][]*Worker
	mu          sync.RWMutex
}

// Worker represents a task execution worker
type Worker struct {
	id       string
	taskType TaskType
	status   WorkerStatus
	taskChan chan *Task
	stopChan chan struct{}
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(config ResourceConfig, scheduler *ScheduledTasks) *WorkerPool {
	wp := &WorkerPool{
		config:      config,
		taskQueue:   make(chan *Task, config.MaxWorkers*2),
		scheduler:   scheduler,
		stopChan:    make(chan struct{}),
		workerTypes: make(map[TaskType][]*Worker),
	}

	// Initialize worker types
	wp.initWorkerTypes()
	return wp
}

// initWorkerTypes initializes worker pools for different task types
func (wp *WorkerPool) initWorkerTypes() {
	// Initialize image generation workers
	wp.workerTypes[TaskTypeImageGen] = make([]*Worker, wp.config.MaxWorkers/3)
	for i := range wp.workerTypes[TaskTypeImageGen] {
		wp.workerTypes[TaskTypeImageGen][i] = newWorker(fmt.Sprintf("img-%d", i), TaskTypeImageGen)
	}

	// Initialize video generation workers
	wp.workerTypes[TaskTypeVideoGen] = make([]*Worker, wp.config.MaxWorkers/3)
	for i := range wp.workerTypes[TaskTypeVideoGen] {
		wp.workerTypes[TaskTypeVideoGen][i] = newWorker(fmt.Sprintf("vid-%d", i), TaskTypeVideoGen)
	}

	// Initialize scheduled task workers
	wp.workerTypes[TaskTypeScheduled] = make([]*Worker, wp.config.MaxWorkers/3)
	for i := range wp.workerTypes[TaskTypeScheduled] {
		wp.workerTypes[TaskTypeScheduled][i] = newWorker(fmt.Sprintf("sch-%d", i), TaskTypeScheduled)
	}
}

// Start starts the worker pool
func (wp *WorkerPool) Start() {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	// Start all workers
	for _, workers := range wp.workerTypes {
		for _, worker := range workers {
			go worker.start()
		}
	}

	// Start task distribution
	go wp.distributeItems()
}

// Stop stops the worker pool
func (wp *WorkerPool) Stop() {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	close(wp.stopChan)
	for _, workers := range wp.workerTypes {
		for _, worker := range workers {
			worker.stop()
		}
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
	wp.mu.Lock()
	defer wp.mu.Unlock()

	workers := wp.workerTypes[task.Type]
	if workers == nil || len(workers) == 0 {
		task.Error = fmt.Errorf("no workers available for task type: %v", task.Type)
		if task.Callback != nil {
			task.Callback.OnError(task.Error)
		}
		return
	}

	// 查找空闲工作者
	for _, worker := range workers {
		if worker.status == WorkerStatusIdle {
			worker.assignTask(task)
			return // 任务已分配
		}
	}

	// 没有空闲工作者，等待一会再重新入队
	task.Status = TaskStatusPending

	// 重要：在锁外执行重新排队操作，避免死锁
	go func() {
		time.Sleep(100 * time.Millisecond)
		wp.requeueTask(task)
	}()
}

// newWorker creates a new worker
func newWorker(id string, taskType TaskType) *Worker {
	return &Worker{
		id:       id,
		taskType: taskType,
		status:   WorkerStatusIdle,
		taskChan: make(chan *Task),
		stopChan: make(chan struct{}),
	}
}

// start starts the worker
func (w *Worker) start() {
	for {
		select {
		case <-w.stopChan:
			return
		case task := <-w.taskChan:
			w.status = WorkerStatusBusy
			task.Execute()
			w.status = WorkerStatusIdle
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
	w.taskChan <- task
}
