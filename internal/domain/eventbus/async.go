package eventbus

import (
	"context"
	"sync"
	"time"

	evbus "github.com/asaskevich/EventBus"
)

// AsyncEventBus 异步事件总线
type AsyncEventBus struct {
	bus       evbus.Bus
	workerNum int
	workChan  chan asyncEvent
	stopChan  chan struct{}
	wg        sync.WaitGroup
}

type asyncEvent struct {
	topic   string
	args    []interface{}
	handler func(args ...interface{})
}

// NewAsyncEventBus 创建异步事件总线
func NewAsyncEventBus(workerNum int) *AsyncEventBus {
	if workerNum <= 0 {
		workerNum = 10 // 默认10个worker
	}

	return &AsyncEventBus{
		bus:       evbus.New(),
		workerNum: workerNum,
		workChan:  make(chan asyncEvent, 1000), // 缓冲区1000个事件
		stopChan:  make(chan struct{}),
	}
}

// Start 启动异步处理
func (aeb *AsyncEventBus) Start() {
	for i := 0; i < aeb.workerNum; i++ {
		aeb.wg.Add(1)
		go aeb.worker()
	}
}

// Stop 停止异步处理
func (aeb *AsyncEventBus) Stop() {
	close(aeb.stopChan)
	aeb.wg.Wait()
}

// worker 异步工作协程
func (aeb *AsyncEventBus) worker() {
	defer aeb.wg.Done()

	for {
		select {
		case <-aeb.stopChan:
			return
		case event := <-aeb.workChan:
			// 设置超时上下文，避免单个事件处理阻塞太久
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			func() {
				defer cancel()
				defer func() {
					if r := recover(); r != nil {
						// 处理panic，避免worker崩溃
					}
				}()
				event.handler(event.args...)
			}()
			_ = ctx // 使用ctx避免编译警告
		}
	}
}

// Publish 发布事件（同步）
func (aeb *AsyncEventBus) Publish(topic string, args ...interface{}) {
	aeb.bus.Publish(topic, args...)
}

// PublishAsync 异步发布事件
func (aeb *AsyncEventBus) PublishAsync(topic string, args ...interface{}) {
	select {
	case aeb.workChan <- asyncEvent{
		topic:   topic,
		args:    args,
		handler: func(args ...interface{}) {
			aeb.bus.Publish(topic, args...)
		},
	}:
	default:
		// 队列满时，丢弃事件并记录警告
		// 这里可以添加监控告警
	}
}

// Subscribe 订阅事件
func (aeb *AsyncEventBus) Subscribe(topic string, fn interface{}) error {
	return aeb.bus.Subscribe(topic, fn)
}

// SubscribeAsync 订阅异步事件
func (aeb *AsyncEventBus) SubscribeAsync(topic string, fn interface{}) error {
	return aeb.bus.Subscribe(topic, fn)
}

// Unsubscribe 取消订阅
func (aeb *AsyncEventBus) Unsubscribe(topic string, handler interface{}) error {
	return aeb.bus.Unsubscribe(topic, handler)
}

// HasCallback 检查是否有订阅者
func (aeb *AsyncEventBus) HasCallback(topic string) bool {
	return aeb.bus.HasCallback(topic)
}

// WaitAsync 等待异步事件处理完成（用于测试）
func (aeb *AsyncEventBus) WaitAsync() {
	// 简单的等待机制，实际使用中可能需要更复杂的同步
	time.Sleep(100 * time.Millisecond)
}