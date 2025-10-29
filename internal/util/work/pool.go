package work

import (
	"errors"
	"sync"
)

// Job represents a task to be executed.
type Job interface{}

// JobHandler defines the function signature for handling a job.
type JobHandler func(job Job) error

// Pool is a worker pool that can be used to execute jobs concurrently.
type Pool struct {
	jobQueue chan Job
	handler  JobHandler
	wg       sync.WaitGroup
	mu       sync.Mutex
	stopped  bool
}

// New creates a new worker pool with the given number of workers and queue size.
func NewWorkPool(numWorkers int, queueSize int, handler JobHandler) *Pool {
	p := &Pool{
		jobQueue: make(chan Job, queueSize),
		handler:  handler,
	}
	p.wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go p.worker()
	}
	return p
}

// Submit submits a job to the worker pool.
func (p *Pool) Submit(job Job) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopped {
		return errors.New("pool is stopped")
	}

	p.jobQueue <- job
	return nil
}

// Stop stops the worker pool and waits for all workers to finish.
func (p *Pool) Stop() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	p.mu.Unlock()

	close(p.jobQueue)
	p.wg.Wait()
}

func (p *Pool) worker() {
	defer func() {
		if r := recover(); r != nil {
			// Handle panic, could log error here
		}
		p.wg.Done()
	}()

	for job := range p.jobQueue {
		p.handler(job)
	}
}