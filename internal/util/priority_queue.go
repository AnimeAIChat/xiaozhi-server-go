package util

import (
	"container/heap"
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrPriorityQueueClosed = errors.New("priority queue closed")
	ErrPriorityQueueEmpty  = errors.New("priority queue empty")
)

// PriorityItem represents an item with priority
type PriorityItem[T any] struct {
	Value    T
	Priority int // Higher number means higher priority
	Index    int // Used by heap interface
}

// PriorityQueue implements a priority queue using heap
type PriorityQueue[T any] struct {
	items  []*PriorityItem[T]
	mu     sync.Mutex
	closed bool
}

// NewPriorityQueue creates a new priority queue
func NewPriorityQueue[T any]() *PriorityQueue[T] {
	pq := &PriorityQueue[T]{
		items: make([]*PriorityItem[T], 0),
	}
	heap.Init(pq)
	return pq
}

// Len implements heap.Interface
func (pq *PriorityQueue[T]) Len() int { return len(pq.items) }

// Less implements heap.Interface (higher priority first)
func (pq *PriorityQueue[T]) Less(i, j int) bool {
	return pq.items[i].Priority > pq.items[j].Priority
}

// Swap implements heap.Interface
func (pq *PriorityQueue[T]) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
	pq.items[i].Index = i
	pq.items[j].Index = j
}

// Push implements heap.Interface
func (pq *PriorityQueue[T]) Push(x interface{}) {
	n := len(pq.items)
	item := x.(*PriorityItem[T])
	item.Index = n
	pq.items = append(pq.items, item)
}

// Pop implements heap.Interface
func (pq *PriorityQueue[T]) Pop() interface{} {
	old := pq.items
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.Index = -1
	pq.items = old[0 : n-1]
	return item
}

// Push adds an item to the priority queue
func (pq *PriorityQueue[T]) PushItem(value T, priority int) error {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if pq.closed {
		return ErrPriorityQueueClosed
	}

	item := &PriorityItem[T]{
		Value:    value,
		Priority: priority,
	}
	heap.Push(pq, item)
	return nil
}

// Pop removes and returns the highest priority item
func (pq *PriorityQueue[T]) PopItem(ctx context.Context, timeout time.Duration) (T, error) {
	var zero T

	pq.mu.Lock()
	if pq.closed {
		pq.mu.Unlock()
		return zero, ErrPriorityQueueClosed
	}
	pq.mu.Unlock()

	// Use a channel to signal when an item is available
	resultChan := make(chan T, 1)
	errorChan := make(chan error, 1)

	go func() {
		pq.mu.Lock()
		defer pq.mu.Unlock()

		if pq.closed {
			errorChan <- ErrPriorityQueueClosed
			return
		}

		if len(pq.items) == 0 {
			errorChan <- ErrPriorityQueueEmpty
			return
		}

		item := heap.Pop(pq).(*PriorityItem[T])
		resultChan <- item.Value
	}()

	if timeout < 0 {
		// Non-blocking
		select {
		case v := <-resultChan:
			return v, nil
		case err := <-errorChan:
			return zero, err
		default:
			return zero, ErrPriorityQueueEmpty
		}
	} else if timeout == 0 {
		// Blocking
		select {
		case v := <-resultChan:
			return v, nil
		case err := <-errorChan:
			return zero, err
		case <-ctx.Done():
			return zero, ctx.Err()
		}
	} else {
		// With timeout
		select {
		case v := <-resultChan:
			return v, nil
		case err := <-errorChan:
			return zero, err
		case <-time.After(timeout):
			return zero, errors.New("priority queue pop timeout")
		case <-ctx.Done():
			return zero, ctx.Err()
		}
	}
}

// Close closes the priority queue
func (pq *PriorityQueue[T]) Close() {
	pq.mu.Lock()
	pq.closed = true
	pq.mu.Unlock()
}

// IsEmpty checks if the queue is empty
func (pq *PriorityQueue[T]) IsEmpty() bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	return len(pq.items) == 0
}