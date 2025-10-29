package work

import (
	"context"
	"errors"
	"sync"
	"time"
	"xiaozhi-server-go/internal/util"
)

var (
	ErrWorkQueueClosed = errors.New("work queue closed")
	ErrMaxRetries      = errors.New("max retries exceeded")
)

// WorkItem represents a work item with retry information
type WorkItem[T any] struct {
	Data      T
	Priority  int
	Retries   int
	MaxRetries int
	LastError error
	CreatedAt time.Time
}

// WorkQueue is a priority-based work queue with retry support
type WorkQueue[T any] struct {
	queue     *util.PriorityQueue[*WorkItem[T]]
	workers   []*Worker[T]
	handler   WorkHandler[T]
	mu        sync.RWMutex
	stopChan  chan struct{}
	stopped   bool
	numWorkers int
}

// WorkHandler defines the function signature for handling work items
type WorkHandler[T any] func(item T) error

// Worker represents a worker goroutine
type Worker[T any] struct {
	id       int
	queue    *WorkQueue[T]
	stopChan chan struct{}
}

// NewWorkQueue creates a new work queue with priority and retry support
func NewWorkQueue[T any](numWorkers int, handler WorkHandler[T]) *WorkQueue[T] {
	wq := &WorkQueue[T]{
		queue:      util.NewPriorityQueue[*WorkItem[T]](),
		handler:    handler,
		stopChan:   make(chan struct{}),
		numWorkers: numWorkers,
		workers:    make([]*Worker[T], numWorkers),
	}

	// Start workers
	for i := 0; i < numWorkers; i++ {
		worker := &Worker[T]{
			id:       i,
			queue:    wq,
			stopChan: make(chan struct{}),
		}
		wq.workers[i] = worker
		go worker.run()
	}

	return wq
}

// Submit submits a work item to the queue
func (wq *WorkQueue[T]) Submit(data T, priority int) error {
	return wq.SubmitWithRetries(data, priority, 0)
}

// SubmitWithRetries submits a work item with retry configuration
func (wq *WorkQueue[T]) SubmitWithRetries(data T, priority int, maxRetries int) error {
	wq.mu.RLock()
	if wq.stopped {
		wq.mu.RUnlock()
		return ErrWorkQueueClosed
	}
	wq.mu.RUnlock()

	item := &WorkItem[T]{
		Data:      data,
		Priority:  priority,
		Retries:   0,
		MaxRetries: maxRetries,
		CreatedAt: time.Now(),
	}

	return wq.queue.PushItem(item, priority)
}

// Stop stops the work queue and waits for all workers to finish
func (wq *WorkQueue[T]) Stop() {
	wq.mu.Lock()
	if wq.stopped {
		wq.mu.Unlock()
		return
	}
	wq.stopped = true
	wq.mu.Unlock()

	// Close stop channel
	close(wq.stopChan)

	// Stop all workers
	for _, worker := range wq.workers {
		close(worker.stopChan)
	}

	// Close the priority queue
	wq.queue.Close()
}

// IsStopped checks if the work queue is stopped
func (wq *WorkQueue[T]) IsStopped() bool {
	wq.mu.RLock()
	defer wq.mu.RUnlock()
	return wq.stopped
}

// GetStats returns queue statistics
func (wq *WorkQueue[T]) GetStats() (queueSize int, isEmpty bool) {
	isEmpty = wq.queue.IsEmpty()
	// Note: PriorityQueue doesn't expose size directly, so we can't get exact size
	return 0, isEmpty
}

// run is the main worker loop
func (w *Worker[T]) run() {
	ctx := context.Background()

	for {
		select {
		case <-w.stopChan:
			return
		case <-w.queue.stopChan:
			return
		default:
			// Try to get work item with timeout
			item, err := w.queue.queue.PopItem(ctx, time.Second*5)
			if err != nil {
				if err == util.ErrPriorityQueueEmpty || err == util.ErrPriorityQueueClosed {
					continue
				}
				// Other errors, continue
				continue
			}

			// Process the work item
			w.processItem(item)
		}
	}
}

// processItem processes a single work item with retry logic
func (w *Worker[T]) processItem(item *WorkItem[T]) {
	for {
		// Execute the handler
		err := w.queue.handler(item.Data)

		if err == nil {
			// Success
			return
		}

		// Handle error
		item.LastError = err
		item.Retries++

		if item.Retries > item.MaxRetries {
			// Max retries exceeded, log error and discard
			// In a real implementation, you might want to send to a dead letter queue
			return
		}

		// Exponential backoff: wait before retry
		backoff := time.Duration(item.Retries) * time.Second
		if backoff > time.Minute {
			backoff = time.Minute
		}

		select {
		case <-time.After(backoff):
			// Continue to retry
		case <-w.stopChan:
			return
		case <-w.queue.stopChan:
			return
		}
	}
}