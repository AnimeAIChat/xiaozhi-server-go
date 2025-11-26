package notifier

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"xiaozhi-server-go/internal/contracts/config"
	"xiaozhi-server-go/internal/utils"
)

// ChangeNotifier 配置变更通知器实现
type ChangeNotifier struct {
	subscribers map[string][]config.ConfigChangeSubscriber
	patterns    map[string]*regexp.Regexp
	mutex       sync.RWMutex
	logger      *utils.Logger
	eventQueue  chan config.ConfigChangeEvent
	workers     int
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewChangeNotifier 创建配置变更通知器
func NewChangeNotifier(logger *utils.Logger, workers int) *ChangeNotifier {
	if logger == nil {
		logger = utils.DefaultLogger
	}

	if workers <= 0 {
		workers = 3 // 默认3个工作协程
	}

	ctx, cancel := context.WithCancel(context.Background())

	notifier := &ChangeNotifier{
		subscribers: make(map[string][]config.ConfigChangeSubscriber),
		patterns:    make(map[string]*regexp.Regexp),
		mutex:       sync.RWMutex{},
		logger:      logger,
		eventQueue:  make(chan config.ConfigChangeEvent, 100),
		workers:     workers,
		ctx:         ctx,
		cancel:      cancel,
	}

	// 启动工作协程
	notifier.startWorkers()

	return notifier
}

// Subscribe 订阅配置变更
func (cn *ChangeNotifier) Subscribe(pattern string, subscriber config.ConfigChangeSubscriber) error {
	if subscriber == nil {
		return fmt.Errorf("subscriber cannot be nil")
	}

	if subscriber.GetID() == "" {
		return fmt.Errorf("subscriber ID cannot be empty")
	}

	if pattern == "" {
		pattern = "*" // 默认订阅所有变更
	}

	cn.mutex.Lock()
	defer cn.mutex.Unlock()

	// 编译正则表达式
	regex, err := cn.compilePattern(pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern '%s': %w", pattern, err)
	}

	// 添加到订阅列表
	subscriberList := cn.subscribers[subscriber.GetID()]
	if subscriberList == nil {
		subscriberList = make([]config.ConfigChangeSubscriber, 0)
	}

	// 检查是否已经存在
	for _, existing := range subscriberList {
		if existing.GetID() == subscriber.GetID() {
			return fmt.Errorf("subscriber '%s' already exists", subscriber.GetID())
		}
	}

	cn.subscribers[subscriber.GetID()] = append(subscriberList, subscriber)
	cn.patterns[subscriber.GetID()] = regex

	cn.logger.InfoTag("ConfigNotifier", "订阅者 '%s' 已订阅模式: %s", subscriber.GetID(), pattern)
	return nil
}

// Unsubscribe 取消订阅
func (cn *ChangeNotifier) Unsubscribe(pattern string, subscriber config.ConfigChangeSubscriber) error {
	if subscriber == nil {
		return fmt.Errorf("subscriber cannot be nil")
	}

	cn.mutex.Lock()
	defer cn.mutex.Unlock()

	subscriberID := subscriber.GetID()
	subscriberList, exists := cn.subscribers[subscriberID]
	if !exists {
		return fmt.Errorf("subscriber '%s' not found", subscriberID)
	}

	// 查找并移除指定的订阅者
	newList := make([]config.ConfigChangeSubscriber, 0, len(subscriberList)-1)
	found := false

	for _, existing := range subscriberList {
		if existing.GetID() == subscriberID && existing.GetFilter() == pattern {
			found = true
			continue
		}
		newList = append(newList, existing)
	}

	if !found {
		return fmt.Errorf("subscription for pattern '%s' not found", pattern)
	}

	if len(newList) == 0 {
		delete(cn.subscribers, subscriberID)
		delete(cn.patterns, subscriberID)
	} else {
		cn.subscribers[subscriberID] = newList
	}

	cn.logger.InfoTag("ConfigNotifier", "订阅者 '%s' 已取消订阅模式: %s", subscriberID, pattern)
	return nil
}

// Notify 通知配置变更
func (cn *ChangeNotifier) Notify(event config.ConfigChangeEvent) error {
	select {
	case cn.eventQueue <- event:
		cn.logger.DebugTag("ConfigNotifier", "配置变更事件已入队: %s = %v", event.Key, event.NewValue)
		return nil
	default:
		cn.logger.WarnTag("ConfigNotifier", "事件队列已满，丢弃配置变更事件: %s", event.Key)
		return fmt.Errorf("event queue is full")
	}
}

// GetSubscribers 获取订阅者列表
func (cn *ChangeNotifier) GetSubscribers() map[string][]config.ConfigChangeSubscriber {
	cn.mutex.RLock()
	defer cn.mutex.RUnlock()

	result := make(map[string][]config.ConfigChangeSubscriber)
	for id, subscribers := range cn.subscribers {
		// 创建副本
		subscriberList := make([]config.ConfigChangeSubscriber, len(subscribers))
		copy(subscriberList, subscribers)
		result[id] = subscriberList
	}

	return result
}

// Close 关闭通知器
func (cn *ChangeNotifier) Close() error {
	cn.cancel()
	close(cn.eventQueue)
	cn.logger.InfoTag("ConfigNotifier", "配置变更通知器已关闭")
	return nil
}

// 私有方法

// startWorkers 启动工作协程
func (cn *ChangeNotifier) startWorkers() {
	for i := 0; i < cn.workers; i++ {
		go cn.worker(i)
	}
	cn.logger.DebugTag("ConfigNotifier", "已启动 %d 个事件处理工作协程", cn.workers)
}

// worker 事件处理工作协程
func (cn *ChangeNotifier) worker(workerID int) {
	cn.logger.DebugTag("ConfigNotifier-Worker%d", "事件处理工作协程已启动", workerID)

	for {
		select {
		case <-cn.ctx.Done():
			cn.logger.DebugTag("ConfigNotifier-Worker%d", "事件处理工作协程已停止", workerID)
			return

		case event, ok := <-cn.eventQueue:
			if !ok {
				cn.logger.DebugTag("ConfigNotifier-Worker%d", "事件通道已关闭", workerID)
				return
			}

			cn.processEvent(workerID, event)
		}
	}
}

// processEvent 处理单个配置变更事件
func (cn *ChangeNotifier) processEvent(workerID int, event config.ConfigChangeEvent) {
	cn.logger.DebugTag("ConfigNotifier-Worker%d", "处理配置变更事件: %s", workerID, event.Key)

	// 获取订阅者列表
	subscribers := cn.getMatchingSubscribers(event.Key)

	if len(subscribers) == 0 {
		cn.logger.DebugTag("ConfigNotifier-Worker%d", "没有订阅者匹配配置项: %s", workerID, event.Key)
		return
	}

	// 通知所有匹配的订阅者
	for _, subscriber := range subscribers {
		if err := cn.notifySubscriber(workerID, subscriber, event); err != nil {
			cn.logger.ErrorTag("ConfigNotifier-Worker%d", "通知订阅者 '%s' 失败: %v",
				workerID, subscriber.GetID(), err)
		}
	}
}

// getMatchingSubscribers 获取匹配的订阅者
func (cn *ChangeNotifier) getMatchingSubscribers(key string) []config.ConfigChangeSubscriber {
	cn.mutex.RLock()
	defer cn.mutex.RUnlock()

	var matchingSubscribers []config.ConfigChangeSubscriber

	for subscriberID, subscriberList := range cn.subscribers {
		pattern, exists := cn.patterns[subscriberID]
		if !exists {
			continue
		}

		// 检查是否匹配模式
		if pattern.MatchString(key) {
			matchingSubscribers = append(matchingSubscribers, subscriberList...)
		}
	}

	return matchingSubscribers
}

// notifySubscriber 通知单个订阅者
func (cn *ChangeNotifier) notifySubscriber(workerID int, subscriber config.ConfigChangeSubscriber, event config.ConfigChangeEvent) error {
	ctx, cancel := context.WithTimeout(cn.ctx, 30*time.Second)
	defer cancel()

	startTime := time.Now()

	err := subscriber.OnConfigChange(ctx, event)
	duration := time.Since(startTime)

	if err != nil {
		cn.logger.ErrorTag("ConfigNotifier-Worker%d", "订阅者 '%s' 处理配置变更失败 (耗时: %v): %v",
			workerID, subscriber.GetID(), duration, err)
		return err
	}

	if subscriber.IsAsync() {
		cn.logger.DebugTag("ConfigNotifier-Worker%d", "订阅者 '%s' 异步处理完成 (耗时: %v)",
			workerID, subscriber.GetID(), duration)
	} else {
		cn.logger.DebugTag("ConfigNotifier-Worker%d", "订阅者 '%s' 同步处理完成 (耗时: %v)",
			workerID, subscriber.GetID(), duration)
	}

	return nil
}

// compilePattern 编译模式
func (cn *ChangeNotifier) compilePattern(pattern string) (*regexp.Regexp, error) {
	// 转换通配符模式为正则表达式
	regexPattern := strings.ReplaceAll(pattern, ".", "\\.")
	regexPattern = strings.ReplaceAll(regexPattern, "*", ".*")
	regexPattern = "^" + regexPattern + "$"

	return regexp.Compile(regexPattern)
}

// SubscriberConfig 订阅者配置
type SubscriberConfig struct {
	ID         string
	Filter     string
	Async      bool
	Timeout    time.Duration
	RetryCount int
}

// DefaultConfigSubscriber 默认配置变更订阅者实现
type DefaultConfigSubscriber struct {
	id      string
	filter  string
	async   bool
	handler func(context.Context, config.ConfigChangeEvent) error
}

// NewDefaultConfigSubscriber 创建默认配置变更订阅者
func NewDefaultConfigSubscriber(id, filter string, async bool, handler func(context.Context, config.ConfigChangeEvent) error) *DefaultConfigSubscriber {
	return &DefaultConfigSubscriber{
		id:      id,
		filter:  filter,
		async:   async,
		handler: handler,
	}
}

// GetID 获取订阅者ID
func (s *DefaultConfigSubscriber) GetID() string {
	return s.id
}

// OnConfigChange 处理配置变更事件
func (s *DefaultConfigSubscriber) OnConfigChange(ctx context.Context, event config.ConfigChangeEvent) error {
	if s.handler == nil {
		return nil
	}

	return s.handler(ctx, event)
}

// GetFilter 获取变更过滤器
func (s *DefaultConfigSubscriber) GetFilter() string {
	return s.filter
}

// IsAsync 是否异步处理
func (s *DefaultConfigSubscriber) IsAsync() bool {
	return s.async
}

// LoggingSubscriber 日志记录订阅者
type LoggingSubscriber struct {
	id     string
	filter string
	logger *utils.Logger
	async  bool
}

// NewLoggingSubscriber 创建日志记录订阅者
func NewLoggingSubscriber(id, filter string, logger *utils.Logger, async bool) *LoggingSubscriber {
	if logger == nil {
		logger = utils.DefaultLogger
	}

	return &LoggingSubscriber{
		id:     id,
		filter: filter,
		logger: logger,
		async:  async,
	}
}

// GetID 获取订阅者ID
func (ls *LoggingSubscriber) GetID() string {
	return ls.id
}

// OnConfigChange 处理配置变更事件
func (ls *LoggingSubscriber) OnConfigChange(ctx context.Context, event config.ConfigChangeEvent) error {
	if event.NewValue != nil {
		ls.logger.InfoTag("配置变更订阅者[%s]", "配置已更新: %s = %v (来源: %s)",
			ls.id, event.Key, event.NewValue, event.Source)
	} else {
		ls.logger.InfoTag("配置变更订阅者[%s]", "配置已删除: %s (来源: %s)",
			ls.id, event.Key, event.Source)
	}
	return nil
}

// GetFilter 获取变更过滤器
func (ls *LoggingSubscriber) GetFilter() string {
	return ls.filter
}

// IsAsync 是否异步处理
func (ls *LoggingSubscriber) IsAsync() bool {
	return ls.async
}