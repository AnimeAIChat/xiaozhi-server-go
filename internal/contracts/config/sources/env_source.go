package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"xiaozhi-server-go/internal/contracts/config"
)

// EnvSource 环境变量配置源
// 支持从环境变量加载配置，支持嵌套结构和类型转换
type EnvSource struct {
	name      string
	prefix    string
	priority  int
	watchMode bool

	// 监听相关
	eventChan     chan config.ConfigChangeEvent
	pollInterval  time.Duration
	lastSnapshot  map[string]string
	mutex         sync.RWMutex

	// 缓存
	cachedConfig map[string]interface{}
	cachedTime   time.Time
	ttl          time.Duration
}

// EnvSourceOption 环境变量配置源选项
type EnvSourceOption func(*EnvSource)

// NewEnvSource 创建环境变量配置源
func NewEnvSource(options ...EnvSourceOption) (*EnvSource, error) {
	source := &EnvSource{
		name:         "env",
		prefix:       "XIAOZHI_",
		priority:     80, // 环境变量通常有较高优先级
		watchMode:    true,
		eventChan:    make(chan config.ConfigChangeEvent, 10),
		pollInterval: 5 * time.Second, // 每5秒检查一次变化
		ttl:          1 * time.Minute,  // 环境变量缓存1分钟
		lastSnapshot: make(map[string]string),
	}

	// 应用选项
	for _, option := range options {
		option(source)
	}

	// 初始加载
	if err := source.initialLoad(); err != nil {
		return nil, fmt.Errorf("failed to initial load: %w", err)
	}

	return source, nil
}

// WithPrefix 设置环境变量前缀
func WithPrefix(prefix string) EnvSourceOption {
	return func(es *EnvSource) {
		if prefix != "" {
			es.prefix = strings.ToUpper(prefix) + "_"
		}
	}
}

// WithEnvPriority 设置优先级
func WithEnvPriority(priority int) EnvSourceOption {
	return func(es *EnvSource) {
		es.priority = priority
	}
}

// WithEnvWatchMode 设置监听模式
func WithEnvWatchMode(enable bool, pollInterval time.Duration) EnvSourceOption {
	return func(es *EnvSource) {
		es.watchMode = enable
		if pollInterval > 0 {
			es.pollInterval = pollInterval
		}
	}
}

// WithEnvTTL 设置缓存TTL
func WithEnvTTL(ttl time.Duration) EnvSourceOption {
	return func(es *EnvSource) {
		es.ttl = ttl
	}
}

// GetName 获取配置源名称
func (es *EnvSource) GetName() string {
	return es.name
}

// GetPriority 获取配置源优先级
func (es *EnvSource) GetPriority() int {
	return es.priority
}

// Load 加载配置数据
func (es *EnvSource) Load(ctx context.Context) (map[string]interface{}, error) {
	es.mutex.RLock()
	defer es.mutex.RUnlock()

	// 检查缓存是否有效
	if es.cachedConfig != nil && time.Since(es.cachedTime) < es.ttl {
		return es.copyConfig(es.cachedConfig), nil
	}

	// 重新加载环境变量
	config, err := es.loadFromEnv()
	if err != nil {
		return nil, err
	}

	// 更新缓存
	es.cachedConfig = config
	es.cachedTime = time.Now()

	return es.copyConfig(config), nil
}

// Watch 监听配置变化
func (es *EnvSource) Watch(ctx context.Context) (<-chan config.ConfigChangeEvent, error) {
	if !es.watchMode {
		return nil, fmt.Errorf("watch mode is disabled")
	}

	// 启动环境变量监听协程
	go es.watchLoop(ctx)

	return es.eventChan, nil
}

// IsAvailable 检查配置源是否可用
func (es *EnvSource) IsAvailable(ctx context.Context) bool {
	// 环境变量总是可用的
	return true
}

// Close 关闭配置源
func (es *EnvSource) Close() error {
	es.mutex.Lock()
	defer es.mutex.Unlock()

	if es.eventChan != nil {
		close(es.eventChan)
		es.eventChan = nil
	}

	return nil
}

// 私有方法

// initialLoad 初始加载
func (es *EnvSource) initialLoad() error {
	config, err := es.loadFromEnv()
	if err != nil {
		return err
	}

	es.mutex.Lock()
	es.cachedConfig = config
	es.cachedTime = time.Now()

	// 创建当前环境变量快照
	es.lastSnapshot = es.createEnvSnapshot()
	es.mutex.Unlock()

	return nil
}

// loadFromEnv 从环境变量加载配置
func (es *EnvSource) loadFromEnv() (map[string]interface{}, error) {
	config := make(map[string]interface{})

	// 遍历所有环境变量
	for _, envLine := range os.Environ() {
		if !strings.HasPrefix(envLine, es.prefix) {
			continue
		}

		parts := strings.SplitN(envLine, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		// 移除前缀
		configKey := strings.ToLower(strings.TrimPrefix(key, es.prefix))

		// 转换嵌套键（例如：SERVER_PORT -> server.port）
		nestedKey := strings.ReplaceAll(configKey, "_", ".")

		// 类型推断和转换
		convertedValue, err := es.convertValue(value)
		if err != nil {
			// 转换失败时使用字符串值
			convertedValue = value
		}

		// 设置嵌套配置
		es.setNestedValue(config, nestedKey, convertedValue)
	}

	return config, nil
}

// convertValue 转换环境变量值的类型
func (es *EnvSource) convertValue(value string) (interface{}, error) {
	// 尝试转换为布尔值
	lowerValue := strings.ToLower(value)
	switch lowerValue {
	case "true", "yes", "1", "on", "enabled":
		return true, nil
	case "false", "no", "0", "off", "disabled":
		return false, nil
	}

	// 尝试转换为整数
	if intVal, err := strconv.Atoi(value); err == nil {
		return intVal, nil
	}

	// 尝试转换为浮点数
	if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
		return floatVal, nil
	}

	// 尝试转换为时间间隔
	if duration, err := time.ParseDuration(value); err == nil {
		return duration, nil
	}

	// 尝试解析为JSON
	if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") || strings.HasPrefix(value, "\"") {
		var jsonValue interface{}
		if err := json.Unmarshal([]byte(value), &jsonValue); err == nil {
			return jsonValue, nil
		}
	}

	// 默认返回字符串
	return value, nil
}

// setNestedValue 设置嵌套配置值
func (es *EnvSource) setNestedValue(config map[string]interface{}, key string, value interface{}) {
	parts := strings.Split(key, ".")
	current := config

	for i, part := range parts {
		if i == len(parts)-1 {
			// 最后一个部分，设置值
			current[part] = value
			return
		}

		// 检查是否存在嵌套map
		if next, exists := current[part]; exists {
			if nextMap, ok := next.(map[string]interface{}); ok {
				current = nextMap
			} else {
				// 存在但不是map，创建新的map
				newMap := make(map[string]interface{})
				current[part] = newMap
				current = newMap
			}
		} else {
			// 不存在，创建新的map
			newMap := make(map[string]interface{})
			current[part] = newMap
			current = newMap
		}
	}
}

// createEnvSnapshot 创建当前环境变量快照
func (es *EnvSource) createEnvSnapshot() map[string]string {
	snapshot := make(map[string]string)

	for _, envLine := range os.Environ() {
		if strings.HasPrefix(envLine, es.prefix) {
			snapshot[envLine] = envLine
		}
	}

	return snapshot
}

// watchLoop 监听环境变量变化
func (es *EnvSource) watchLoop(ctx context.Context) {
	ticker := time.NewTicker(es.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			es.checkForChanges()
		}
	}
}

// checkForChanges 检查环境变量变化
func (es *EnvSource) checkForChanges() {
	currentSnapshot := es.createEnvSnapshot()

	es.mutex.RLock()
	lastSnapshot := es.lastSnapshot
	es.mutex.RUnlock()

	// 检测新增和修改的环境变量
	for key, newValue := range currentSnapshot {
		if oldValue, exists := lastSnapshot[key]; !exists {
			// 新增环境变量
			es.handleEnvChange(key, newValue, "")
		} else if oldValue != newValue {
			// 修改环境变量
			es.handleEnvChange(key, newValue, oldValue)
		}
	}

	// 检测删除的环境变量
	for key := range lastSnapshot {
		if _, exists := currentSnapshot[key]; !exists {
			// 删除环境变量
			es.handleEnvChange(key, "", lastSnapshot[key])
		}
	}

	// 更新快照
	es.mutex.Lock()
	es.lastSnapshot = currentSnapshot
	es.mutex.Unlock()
}

// handleEnvChange 处理环境变量变化
func (es *EnvSource) handleEnvChange(envLine, newValue, oldValue string) {
	if !strings.HasPrefix(envLine, es.prefix) {
		return
	}

	// 提取配置键
	parts := strings.SplitN(envLine, "=", 2)
	if len(parts) != 2 {
		return
	}

	envKey := parts[0]
	configKey := strings.ToLower(strings.TrimPrefix(envKey, es.prefix))
	nestedKey := strings.ReplaceAll(configKey, "_", ".")

	// 转换值类型
	var newConfigValue, oldConfigValue interface{}
	var err error

	if newValue != "" {
		newConfigValue, err = es.convertValue(newValue)
		if err != nil {
			newConfigValue = newValue
		}
	}

	if oldValue != "" {
		oldParts := strings.SplitN(oldValue, "=", 2)
		if len(oldParts) == 2 {
			oldConfigValue, err = es.convertValue(oldParts[1])
			if err != nil {
				oldConfigValue = oldParts[1]
			}
		}
	}

	// 发送变更事件
	event := config.ConfigChangeEvent{
		Source:    es.name,
		Key:       nestedKey,
		NewValue:  newConfigValue,
		OldValue:  oldConfigValue,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"env_key": envKey,
		},
	}

	select {
	case es.eventChan <- event:
	default:
		// 事件通道满了，丢弃事件
	}
}

// copyConfig 复制配置（深拷贝）
func (es *EnvSource) copyConfig(config map[string]interface{}) map[string]interface{} {
	if config == nil {
		return nil
	}

	// 使用JSON序列化进行深拷贝
	data, err := json.Marshal(config)
	if err != nil {
		return make(map[string]interface{})
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)
	return result
}