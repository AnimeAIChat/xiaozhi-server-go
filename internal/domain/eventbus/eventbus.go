package eventbus

import (
	"sync"

	evbus "github.com/asaskevich/EventBus"
)

var (
	instance evbus.Bus
	asyncBus *AsyncEventBus
	once     sync.Once
)

// Get 获取同步事件总线实例
func Get() evbus.Bus {
	once.Do(func() {
		instance = New()
		asyncBus = NewAsyncEventBus(10) // 10个worker
		asyncBus.Start()
	})
	return instance
}

// GetAsync 获取异步事件总线实例
func GetAsync() *AsyncEventBus {
	once.Do(func() {
		instance = New()
		asyncBus = NewAsyncEventBus(10) // 10个worker
		asyncBus.Start()
	})
	return asyncBus
}

// New 创建新的同步事件总线
func New() evbus.Bus {
	return evbus.New()
}

// Publish 发布同步事件
func Publish(topic string, args ...interface{}) {
	Get().Publish(topic, args...)
}

// PublishAsync 发布异步事件
func PublishAsync(topic string, args ...interface{}) {
	GetAsync().PublishAsync(topic, args...)
}

// Subscribe 订阅同步事件
func Subscribe(topic string, fn interface{}) error {
	return Get().Subscribe(topic, fn)
}

// SubscribeAsync 订阅异步事件
func SubscribeAsync(topic string, fn interface{}) error {
	return GetAsync().SubscribeAsync(topic, fn)
}

// Shutdown 关闭事件总线
func Shutdown() {
	if asyncBus != nil {
		asyncBus.Stop()
	}
}