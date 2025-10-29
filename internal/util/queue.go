package util

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrQueueClosed = errors.New("queue closed or cleared")
var ErrQueueTimeout = errors.New("queue pop timeout")
var ErrQueueEmpty = errors.New("queue empty (non-blocking pop)")
var ErrQueueCtxDone = errors.New("queue ctx done")

// Queue is a generic, thread-safe queue based on chan.
type Queue[T any] struct {
	mu     sync.Mutex
	ch     chan T
	cap    int
	closed bool
}

// NewQueue creates a new Queue with the given capacity.
func NewQueue[T any](capacity int) *Queue[T] {
	return &Queue[T]{
		ch:  make(chan T, capacity),
		cap: capacity,
	}
}

// Push adds an item to the queue. Returns error if queue is closed.
func (q *Queue[T]) Push(val T) error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return ErrQueueClosed
	}
	ch := q.ch
	q.mu.Unlock()

	select {
	case ch <- val:
		return nil
	default:
		// If full, block until space is available or closed
		select {
		case ch <- val:
			return nil
		case <-time.After(time.Second * 10): // avoid deadlock
			return errors.New("push timeout (10s)")
		}
	}
}

// Pop tries to get an item from the queue.
// ctx: 支持取消，ctx.Done()时立即返回
// timeout=0: block until item或queue cleared
// timeout<0: non-blocking
// timeout>0: wait up to timeout duration
func (q *Queue[T]) Pop(ctx context.Context, timeout time.Duration) (T, error) {
	var zero T
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return zero, ErrQueueClosed
	}
	ch := q.ch
	q.mu.Unlock()

	if timeout < 0 {
		// Non-blocking
		select {
		case v, ok := <-ch:
			if !ok {
				return zero, ErrQueueClosed
			}
			return v, nil
		default:
			return zero, ErrQueueEmpty
		}
	} else if timeout == 0 {
		// Blocking, 支持ctx.Done()
		select {
		case v, ok := <-ch:
			if !ok {
				return zero, ErrQueueClosed
			}
			return v, nil
		case <-ctx.Done():
			return zero, ErrQueueCtxDone
		}
	} else {
		// Timeout, 支持ctx.Done()
		select {
		case v, ok := <-ch:
			if !ok {
				return zero, ErrQueueClosed
			}
			return v, nil
		case <-time.After(timeout):
			return zero, ErrQueueTimeout
		case <-ctx.Done():
			return zero, ctx.Err()
		}
	}
}

// Clear empties the queue and ensures all Pop calls return immediately.
func (q *Queue[T]) Clear() {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	oldCh := q.ch
	q.ch = make(chan T, q.cap)
	close(oldCh)
	q.mu.Unlock()
}

// Close closes the queue permanently. All Push/Pop will error after this.
func (q *Queue[T]) Close() {
	q.mu.Lock()
	if !q.closed {
		q.closed = true
		close(q.ch)
	}
	q.mu.Unlock()
}