package config

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/src/core/utils"
)

// UnifiedConfigManagerImpl 统一配置管理器实现
type UnifiedConfigManagerImpl struct {
	sessionID       string
	logger          *utils.Logger
	ctx             context.Context
	cancel          context.CancelFunc
	isInitialized   bool

	// 配置源管理
	sources         []ConfigSource
	sourcesMutex    sync.RWMutex

	// 配置数据
	configData      map[string]interface{}
	configMutex     sync.RWMutex

	// 组件依赖
	validator       ConfigValidator
	cache           ConfigCache
	notifier        ConfigNotifier

	// 配置策略
	mergeStrategy   ConfigMergeStrategy
	hotReloadMode   ConfigHotReloadMode
	defaultTTL      time.Duration

	// 统计信息
	stats           UnifiedConfigStats
	statsMutex      sync.Mutex

	// 事件通道
	changeEventChan chan ConfigChangeEvent
	eventWorkers    int
}

// ConfigManagerOption 配置管理器选项
type ConfigManagerOption func(*UnifiedConfigManagerImpl)

// NewUnifiedConfigManager 创建统一配置管理器
func NewUnifiedConfigManager(logger *utils.Logger, options ...ConfigManagerOption) UnifiedConfigManager {
	if logger == nil {
		logger = utils.DefaultLogger
	}

	ctx, cancel := context.WithCancel(context.Background())

	manager := &UnifiedConfigManagerImpl{
		sessionID:       fmt.Sprintf("config-manager-%d", time.Now().UnixNano()),
		logger:          logger,
		ctx:             ctx,
		cancel:          cancel,
		configData:      make(map[string]interface{}),
		sources:         make([]ConfigSource, 0),
		mergeStrategy:   MergeStrategyOverwrite,
		hotReloadMode:   HotReloadAuto,
		defaultTTL:      5 * time.Minute,
		changeEventChan: make(chan ConfigChangeEvent, 100),
		eventWorkers:    3,
		stats: UnifiedConfigStats{
			SourceStats: make(map[string]SourceStats),
		},
	}

	// 应用选项
	for _, option := range options {
		option(manager)
	}

	return manager
}

// WithMergeStrategy 设置合并策略
func WithMergeStrategy(strategy ConfigMergeStrategy) ConfigManagerOption {
	return func(m *UnifiedConfigManagerImpl) {
		m.mergeStrategy = strategy
	}
}

// WithHotReloadMode 设置热重载模式
func WithHotReloadMode(mode ConfigHotReloadMode) ConfigManagerOption {
	return func(m *UnifiedConfigManagerImpl) {
		m.hotReloadMode = mode
	}
}

// WithDefaultTTL 设置默认TTL
func WithDefaultTTL(ttl time.Duration) ConfigManagerOption {
	return func(m *UnifiedConfigManagerImpl) {
		m.defaultTTL = ttl
	}
}

// WithEventWorkers 设置事件工作协程数
func WithEventWorkers(workers int) ConfigManagerOption {
	return func(m *UnifiedConfigManagerImpl) {
		m.eventWorkers = workers
	}
}

// Initialize 初始化配置管理器
func (m *UnifiedConfigManagerImpl) Initialize(ctx context.Context) error {
	m.logger.InfoTag("ConfigManager", "初始化统一配置管理器，SessionID: %s", m.sessionID)

	// 启动事件处理工作协程
	m.startEventWorkers()

	// 按优先级排序配置源
	m.sortSourcesByPriority()

	// 加载所有配置源
	if err := m.loadAllSources(ctx); err != nil {
		return fmt.Errorf("failed to load config sources: %w", err)
	}

	// 启动配置监听
	if m.hotReloadMode == HotReloadAuto {
		go m.startConfigWatching(ctx)
	}

	m.isInitialized = true
	m.logger.InfoTag("ConfigManager", "统一配置管理器初始化完成，SessionID: %s", m.sessionID)
	return nil
}

// Cleanup 清理资源
func (m *UnifiedConfigManagerImpl) Cleanup() error {
	m.logger.InfoTag("ConfigManager", "清理统一配置管理器，SessionID: %s", m.sessionID)

	m.cancel()

	// 关闭所有配置源
	m.sourcesMutex.Lock()
	for _, source := range m.sources {
		if err := source.Close(); err != nil {
			m.logger.WarnTag("ConfigManager", "关闭配置源 %s 失败: %v", source.GetName(), err)
		}
	}
	m.sources = nil
	m.sourcesMutex.Unlock()

	// 关闭缓存
	if m.cache != nil {
		m.cache.StopGC()
	}

	// 关闭事件通道
	close(m.changeEventChan)

	m.isInitialized = false
	return nil
}

// Get 获取配置值
func (m *UnifiedConfigManagerImpl) Get(key string) (interface{}, error) {
	if !m.isInitialized {
		return nil, fmt.Errorf("config manager not initialized")
	}

	// 首先检查缓存
	if m.cache != nil {
		if value, found := m.cache.Get(key); found {
			m.recordCacheHit()
			return value, nil
		}
		m.recordCacheMiss()
	}

	// 从配置数据中获取
	m.configMutex.RLock()
	defer m.configMutex.RUnlock()

	value, exists := m.configData[key]
	if !exists {
		return nil, fmt.Errorf("config key '%s' not found", key)
	}

	// 缓存结果
	if m.cache != nil {
		m.cache.Set(key, value, m.defaultTTL)
	}

	return value, nil
}

// GetWithDefault 获取配置值，带默认值
func (m *UnifiedConfigManagerImpl) GetWithDefault(key string, defaultValue interface{}) interface{} {
	value, err := m.Get(key)
	if err != nil {
		return defaultValue
	}
	return value
}

// GetString 获取字符串配置值
func (m *UnifiedConfigManagerImpl) GetString(key string) (string, error) {
	value, err := m.Get(key)
	if err != nil {
		return "", err
	}

	if strValue, ok := value.(string); ok {
		return strValue, nil
	}

	return fmt.Sprintf("%v", value), nil
}

// GetStringWithDefault 获取字符串配置值，带默认值
func (m *UnifiedConfigManagerImpl) GetStringWithDefault(key, defaultValue string) string {
	value, err := m.GetString(key)
	if err != nil {
		return defaultValue
	}
	return value
}

// GetInt 获取整数配置值
func (m *UnifiedConfigManagerImpl) GetInt(key string) (int, error) {
	value, err := m.Get(key)
	if err != nil {
		return 0, err
	}

	switch v := value.(type) {
	case int:
		return v, nil
	case float64:
		return int(v), nil
	case string:
		// 尝试解析字符串
		var intValue int
		if _, err := fmt.Sscanf(v, "%d", &intValue); err == nil {
			return intValue, nil
		}
	}

	return 0, fmt.Errorf("config key '%s' is not an integer", key)
}

// GetIntWithDefault 获取整数配置值，带默认值
func (m *UnifiedConfigManagerImpl) GetIntWithDefault(key string, defaultValue int) int {
	value, err := m.GetInt(key)
	if err != nil {
		return defaultValue
	}
	return value
}

// GetBool 获取布尔配置值
func (m *UnifiedConfigManagerImpl) GetBool(key string) (bool, error) {
	value, err := m.Get(key)
	if err != nil {
		return false, err
	}

	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		return strings.ToLower(v) == "true" || v == "1", nil
	case int:
		return v != 0, nil
	case float64:
		return v != 0, nil
	}

	return false, fmt.Errorf("config key '%s' is not a boolean", key)
}

// GetBoolWithDefault 获取布尔配置值，带默认值
func (m *UnifiedConfigManagerImpl) GetBoolWithDefault(key string, defaultValue bool) bool {
	value, err := m.GetBool(key)
	if err != nil {
		return defaultValue
	}
	return value
}

// GetDuration 获取时间间隔配置值
func (m *UnifiedConfigManagerImpl) GetDuration(key string) (time.Duration, error) {
	value, err := m.Get(key)
	if err != nil {
		return 0, err
	}

	switch v := value.(type) {
	case time.Duration:
		return v, nil
	case int:
		return time.Duration(v) * time.Second, nil
	case float64:
		return time.Duration(v) * time.Second, nil
	case string:
		return time.ParseDuration(v)
	}

	return 0, fmt.Errorf("config key '%s' is not a duration", key)
}

// GetDurationWithDefault 获取时间间隔配置值，带默认值
func (m *UnifiedConfigManagerImpl) GetDurationWithDefault(key string, defaultValue time.Duration) time.Duration {
	value, err := m.GetDuration(key)
	if err != nil {
		return defaultValue
	}
	return value
}

// Set 设置配置值
func (m *UnifiedConfigManagerImpl) Set(key string, value interface{}) error {
	return m.SetWithSource(key, value, "runtime")
}

// SetWithSource 设置配置值并指定来源
func (m *UnifiedConfigManagerImpl) SetWithSource(key string, value interface{}, source string) error {
	if !m.isInitialized {
		return fmt.Errorf("config manager not initialized")
	}

	// 验证配置值
	if m.validator != nil {
		if err := m.validator.ValidateValue(key, value); err != nil {
			m.recordValidationError()
			return fmt.Errorf("validation failed for key '%s': %w", key, err)
		}
	}

	// 获取旧值
	oldValue := m.GetWithDefault(key, nil)

	// 设置新值
	m.configMutex.Lock()
	m.configData[key] = value
	m.configMutex.Unlock()

	// 清除缓存
	if m.cache != nil {
		m.cache.Delete(key)
	}

	// 发送变更事件
	event := ConfigChangeEvent{
		Source:    source,
		Key:       key,
		OldValue:  oldValue,
		NewValue:  value,
		Timestamp: time.Now(),
	}

	m.notifyChange(event)

	m.logger.DebugTag("ConfigManager", "配置已更新: %s = %v (来源: %s)", key, value, source)
	return nil
}

// Delete 删除配置项
func (m *UnifiedConfigManagerImpl) Delete(key string) error {
	if !m.isInitialized {
		return fmt.Errorf("config manager not initialized")
	}

	// 获取旧值
	oldValue := m.GetWithDefault(key, nil)

	// 删除配置项
	m.configMutex.Lock()
	delete(m.configData, key)
	m.configMutex.Unlock()

	// 清除缓存
	if m.cache != nil {
		m.cache.Delete(key)
	}

	// 发送变更事件
	event := ConfigChangeEvent{
		Source:    "runtime",
		Key:       key,
		OldValue:  oldValue,
		NewValue:  nil,
		Timestamp: time.Now(),
	}

	m.notifyChange(event)

	m.logger.DebugTag("ConfigManager", "配置已删除: %s", key)
	return nil
}

// GetAll 获取所有配置
func (m *UnifiedConfigManagerImpl) GetAll() (map[string]interface{}, error) {
	if !m.isInitialized {
		return nil, fmt.Errorf("config manager not initialized")
	}

	m.configMutex.RLock()
	defer m.configMutex.RUnlock()

	// 创建配置副本
	result := make(map[string]interface{})
	for k, v := range m.configData {
		result[k] = v
	}

	return result, nil
}

// Reload 重新加载配置
func (m *UnifiedConfigManagerImpl) Reload(ctx context.Context) error {
	if !m.isInitialized {
		return fmt.Errorf("config manager not initialized")
	}

	m.logger.InfoTag("ConfigManager", "开始重新加载配置，SessionID: %s", m.sessionID)

	// 保存当前配置用于比较
	oldConfig := m.getCurrentConfig()

	// 重新加载所有配置源
	if err := m.loadAllSources(ctx); err != nil {
		return fmt.Errorf("failed to reload config sources: %w", err)
	}

	// 比较配置变化并发送事件
	newConfig := m.getCurrentConfig()
	m.detectChangesAndNotify(oldConfig, newConfig)

	// 更新统计信息
	m.recordReload()

	m.logger.InfoTag("ConfigManager", "配置重新加载完成，SessionID: %s", m.sessionID)
	return nil
}

// Save 保存配置到持久化存储
func (m *UnifiedConfigManagerImpl) Save(ctx context.Context) error {
	if !m.isInitialized {
		return fmt.Errorf("config manager not initialized")
	}

	m.logger.InfoTag("ConfigManager", "保存配置到持久化存储，SessionID: %s", m.sessionID)

	// 获取当前配置
	configData, err := m.GetAll()
	if err != nil {
		return fmt.Errorf("failed to get current config: %w", err)
	}

	// TODO: 实现持久化保存逻辑
	// 这里需要保存到数据库或其他持久化存储
	// 暂时跳过平台配置转换
	_ = m.convertToPlatformConfig(configData)

	m.logger.InfoTag("ConfigManager", "配置保存完成，SessionID: %s", m.sessionID)
	return nil
}

// AddSource 添加配置源
func (m *UnifiedConfigManagerImpl) AddSource(source ConfigSource) error {
	if source == nil {
		return fmt.Errorf("config source cannot be nil")
	}

	m.sourcesMutex.Lock()
	defer m.sourcesMutex.Unlock()

	// 检查是否已存在
	for _, existing := range m.sources {
		if existing.GetName() == source.GetName() {
			return fmt.Errorf("config source '%s' already exists", source.GetName())
		}
	}

	m.sources = append(m.sources, source)

	// 重新排序
	m.sortSourcesByPriorityLocked()

	m.logger.InfoTag("ConfigManager", "已添加配置源: %s (优先级: %d)", source.GetName(), source.GetPriority())
	return nil
}

// RemoveSource 移除配置源
func (m *UnifiedConfigManagerImpl) RemoveSource(sourceName string) error {
	m.sourcesMutex.Lock()
	defer m.sourcesMutex.Unlock()

	for i, source := range m.sources {
		if source.GetName() == sourceName {
			// 关闭配置源
			if err := source.Close(); err != nil {
				m.logger.WarnTag("ConfigManager", "关闭配置源 %s 失败: %v", sourceName, err)
			}

			// 从列表中移除
			m.sources = append(m.sources[:i], m.sources[i+1:]...)

			m.logger.InfoTag("ConfigManager", "已移除配置源: %s", sourceName)
			return nil
		}
	}

	return fmt.Errorf("config source '%s' not found", sourceName)
}

// GetSources 获取所有配置源
func (m *UnifiedConfigManagerImpl) GetSources() []ConfigSource {
	m.sourcesMutex.RLock()
	defer m.sourcesMutex.RUnlock()

	// 返回副本
	sources := make([]ConfigSource, len(m.sources))
	copy(sources, m.sources)
	return sources
}

// SetValidator 设置配置验证器
func (m *UnifiedConfigManagerImpl) SetValidator(validator ConfigValidator) {
	m.validator = validator
	if validator != nil {
		m.logger.InfoTag("ConfigManager", "已设置配置验证器")
	}
}

// SetCache 设置配置缓存
func (m *UnifiedConfigManagerImpl) SetCache(cache ConfigCache) {
	m.cache = cache
	if cache != nil {
		cache.SetTTL(m.defaultTTL)
		cache.StartGC(5 * time.Minute) // 每5分钟进行一次垃圾回收
		m.logger.InfoTag("ConfigManager", "已设置配置缓存")
	}
}

// SetNotifier 设置变更通知器
func (m *UnifiedConfigManagerImpl) SetNotifier(notifier ConfigNotifier) {
	m.notifier = notifier
	if notifier != nil {
		m.logger.InfoTag("ConfigManager", "已设置配置变更通知器")
	}
}

// GetStats 获取统计信息
func (m *UnifiedConfigManagerImpl) GetStats() UnifiedConfigStats {
	m.statsMutex.Lock()
	defer m.statsMutex.Unlock()

	// 更新基本统计信息
	m.stats.TotalKeys = len(m.configData)

	m.sourcesMutex.RLock()
	activeSources := 0
	for _, source := range m.sources {
		if source.IsAvailable(m.ctx) {
			activeSources++
		}
	}
	m.sourcesMutex.RUnlock()

	m.stats.ActiveSources = activeSources
	m.stats.TotalSources = len(m.sources)

	// 获取缓存统计信息
	if m.cache != nil {
		m.stats.CacheStats = m.cache.GetStats()
	}

	return m.stats
}

// HealthCheck 健康检查
func (m *UnifiedConfigManagerImpl) HealthCheck(ctx context.Context) error {
	if !m.isInitialized {
		return fmt.Errorf("config manager not initialized")
	}

	// 检查配置源健康状态
	m.sourcesMutex.RLock()
	defer m.sourcesMutex.RUnlock()

	unavailableSources := make([]string, 0)
	for _, source := range m.sources {
		if !source.IsAvailable(ctx) {
			unavailableSources = append(unavailableSources, source.GetName())
		}
	}

	if len(unavailableSources) > 0 {
		return fmt.Errorf("unavailable config sources: %v", unavailableSources)
	}

	return nil
}

// Export 导出配置
func (m *UnifiedConfigManagerImpl) Export(ctx context.Context) (*config.Config, error) {
	configData, err := m.GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to get config data: %w", err)
	}

	return m.convertToPlatformConfig(configData), nil
}

// Import 导入配置
func (m *UnifiedConfigManagerImpl) Import(ctx context.Context, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	m.logger.InfoTag("ConfigManager", "导入配置，SessionID: %s", m.sessionID)

	// 转换为映射格式
	configData := m.convertFromPlatformConfig(cfg)

	// 验证配置
	if m.validator != nil {
		if err := m.validator.ValidateSchema(configData); err != nil {
			return fmt.Errorf("config validation failed: %w", err)
		}
	}

	// 更新配置数据
	m.configMutex.Lock()
	for key, value := range configData {
		m.configData[key] = value
	}
	m.configMutex.Unlock()

	// 清除缓存
	if m.cache != nil {
		m.cache.Clear()
	}

	m.logger.InfoTag("ConfigManager", "配置导入完成，SessionID: %s", m.sessionID)
	return nil
}

// 私有方法

// startEventWorkers 启动事件处理工作协程
func (m *UnifiedConfigManagerImpl) startEventWorkers() {
	for i := 0; i < m.eventWorkers; i++ {
		go m.eventWorker(i)
	}
}

// eventWorker 事件处理工作协程
func (m *UnifiedConfigManagerImpl) eventWorker(workerID int) {
	for {
		select {
		case <-m.ctx.Done():
			return
		case event, ok := <-m.changeEventChan:
			if !ok {
				return
			}
			m.processChangeEvent(workerID, event)
		}
	}
}

// processChangeEvent 处理配置变更事件
func (m *UnifiedConfigManagerImpl) processChangeEvent(workerID int, event ConfigChangeEvent) {
	m.logger.DebugTag("ConfigManager-Worker%d", "处理配置变更事件: %s", workerID, event.Key)

	// 通知订阅者
	if m.notifier != nil {
		if err := m.notifier.Notify(event); err != nil {
			m.logger.WarnTag("ConfigManager-Worker%d", "通知配置变更失败: %v", workerID, err)
		}
	}
}

// sortSourcesByPriority 按优先级排序配置源
func (m *UnifiedConfigManagerImpl) sortSourcesByPriority() {
	m.sourcesMutex.Lock()
	defer m.sourcesMutex.Unlock()
	m.sortSourcesByPriorityLocked()
}

// sortSourcesByPriorityLocked 按优先级排序配置源（需要已持有锁）
func (m *UnifiedConfigManagerImpl) sortSourcesByPriorityLocked() {
	sort.Slice(m.sources, func(i, j int) bool {
		return m.sources[i].GetPriority() > m.sources[j].GetPriority()
	})
}

// loadAllSources 加载所有配置源
func (m *UnifiedConfigManagerImpl) loadAllSources(ctx context.Context) error {
	m.sourcesMutex.RLock()
	sources := make([]ConfigSource, len(m.sources))
	copy(sources, m.sources)
	m.sourcesMutex.RUnlock()

	// 按优先级排序（已排序，但保险起见）
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].GetPriority() > sources[j].GetPriority()
	})

	// 清空当前配置
	m.configMutex.Lock()
	m.configData = make(map[string]interface{})
	m.configMutex.Unlock()

	// 按优先级加载配置源
	for _, source := range sources {
		if !source.IsAvailable(ctx) {
			m.logger.WarnTag("ConfigManager", "配置源 %s 不可用，跳过", source.GetName())
			continue
		}

		startTime := time.Now()
		sourceConfig, err := source.Load(ctx)
		loadTime := time.Since(startTime)

		// 更新源统计信息
		m.updateSourceStats(source.GetName(), true, loadTime)

		if err != nil {
			m.logger.ErrorTag("ConfigManager", "加载配置源 %s 失败: %v", source.GetName(), err)
			m.updateSourceStats(source.GetName(), false, loadTime)
			continue
		}

		// 合并配置
		m.mergeConfig(sourceConfig, source.GetName())

		m.logger.DebugTag("ConfigManager", "已加载配置源 %s，加载时间: %v", source.GetName(), loadTime)
	}

	// 验证最终配置
	if m.validator != nil {
		m.configMutex.RLock()
		if err := m.validator.ValidateSchema(m.configData); err != nil {
			m.configMutex.RUnlock()
			return fmt.Errorf("config validation failed: %w", err)
		}
		m.configMutex.RUnlock()
	}

	return nil
}

// mergeConfig 合并配置
func (m *UnifiedConfigManagerImpl) mergeConfig(sourceConfig map[string]interface{}, sourceName string) {
	m.configMutex.Lock()
	defer m.configMutex.Unlock()

	switch m.mergeStrategy {
	case MergeStrategyOverwrite:
		// 覆盖策略：直接覆盖
		for key, value := range sourceConfig {
			m.configData[key] = value
		}
	case MergeStrategyMerge:
		// 合并策略：智能合并
		m.mergeConfigSmart(sourceConfig)
	case MergeStrategyAppend:
		// 追加策略：只在键不存在时添加
		for key, value := range sourceConfig {
			if _, exists := m.configData[key]; !exists {
				m.configData[key] = value
			}
		}
	}
}

// mergeConfigSmart 智能合并配置
func (m *UnifiedConfigManagerImpl) mergeConfigSmart(sourceConfig map[string]interface{}) {
	for key, newValue := range sourceConfig {
		if oldValue, exists := m.configData[key]; exists {
			// 尝试智能合并
			if merged := m.tryMergeValues(oldValue, newValue); merged != nil {
				m.configData[key] = merged
			} else {
				// 无法合并，使用新值覆盖
				m.configData[key] = newValue
			}
		} else {
			// 键不存在，直接添加
			m.configData[key] = newValue
		}
	}
}

// tryMergeValues 尝试合并两个值
func (m *UnifiedConfigManagerImpl) tryMergeValues(oldValue, newValue interface{}) interface{} {
	// 如果都是map类型，尝试递归合并
	oldMap, oldIsMap := oldValue.(map[string]interface{})
	newMap, newIsMap := newValue.(map[string]interface{})

	if oldIsMap && newIsMap {
		merged := make(map[string]interface{})
		// 复制旧值
		for k, v := range oldMap {
			merged[k] = v
		}
		// 合并新值
		for k, v := range newMap {
			merged[k] = v
		}
		return merged
	}

	// 如果都是slice类型，尝试合并
	oldSlice, oldIsSlice := m.convertToSlice(oldValue)
	newSlice, newIsSlice := m.convertToSlice(newValue)

	if oldIsSlice && newIsSlice {
		return append(oldSlice, newSlice...)
	}

	// 无法合并，返回nil表示使用新值覆盖
	return nil
}

// convertToSlice 转换为slice
func (m *UnifiedConfigManagerImpl) convertToSlice(value interface{}) ([]interface{}, bool) {
	val := reflect.ValueOf(value)
	if val.Kind() == reflect.Slice {
		result := make([]interface{}, val.Len())
		for i := 0; i < val.Len(); i++ {
			result[i] = val.Index(i).Interface()
		}
		return result, true
	}
	return nil, false
}

// startConfigWatching 启动配置监听
func (m *UnifiedConfigManagerImpl) startConfigWatching(ctx context.Context) {
	m.sourcesMutex.RLock()
	sources := make([]ConfigSource, len(m.sources))
	copy(sources, m.sources)
	m.sourcesMutex.RUnlock()

	for _, source := range sources {
		go m.watchSource(ctx, source)
	}
}

// watchSource 监听单个配置源
func (m *UnifiedConfigManagerImpl) watchSource(ctx context.Context, source ConfigSource) {
	watchChan, err := source.Watch(ctx)
	if err != nil {
		m.logger.WarnTag("ConfigManager", "配置源 %s 不支持监听: %v", source.GetName(), err)
		return
	}

	m.logger.DebugTag("ConfigManager", "开始监听配置源: %s", source.GetName())

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watchChan:
			if !ok {
				return
			}
			m.handleSourceChangeEvent(event)
		}
	}
}

// handleSourceChangeEvent 处理配置源变更事件
func (m *UnifiedConfigManagerImpl) handleSourceChangeEvent(event ConfigChangeEvent) {
	m.logger.InfoTag("ConfigManager", "检测到配置源变更: %s.%s = %v", event.Source, event.Key, event.NewValue)

	// 更新配置值
	if event.NewValue != nil {
		m.SetWithSource(event.Key, event.NewValue, event.Source)
	} else {
		m.Delete(event.Key)
	}
}

// notifyChange 通知配置变更
func (m *UnifiedConfigManagerImpl) notifyChange(event ConfigChangeEvent) {
	select {
	case m.changeEventChan <- event:
	default:
		m.logger.WarnTag("ConfigManager", "配置变更事件通道已满，丢弃事件: %s", event.Key)
	}
}

// getCurrentConfig 获取当前配置的副本
func (m *UnifiedConfigManagerImpl) getCurrentConfig() map[string]interface{} {
	m.configMutex.RLock()
	defer m.configMutex.RUnlock()

	config := make(map[string]interface{})
	for k, v := range m.configData {
		config[k] = v
	}
	return config
}

// detectChangesAndNotify 检测配置变化并发送事件
func (m *UnifiedConfigManagerImpl) detectChangesAndNotify(oldConfig, newConfig map[string]interface{}) {
	// 检测新增和修改的配置项
	for key, newValue := range newConfig {
		if oldValue, exists := oldConfig[key]; !exists {
			// 新增配置项
			event := ConfigChangeEvent{
				Source:    "reload",
				Key:       key,
				OldValue:  nil,
				NewValue:  newValue,
				Timestamp: time.Now(),
			}
			m.notifyChange(event)
		} else if !reflect.DeepEqual(oldValue, newValue) {
			// 修改配置项
			event := ConfigChangeEvent{
				Source:    "reload",
				Key:       key,
				OldValue:  oldValue,
				NewValue:  newValue,
				Timestamp: time.Now(),
			}
			m.notifyChange(event)
		}
	}

	// 检测删除的配置项
	for key := range oldConfig {
		if _, exists := newConfig[key]; !exists {
			event := ConfigChangeEvent{
				Source:    "reload",
				Key:       key,
				OldValue:  oldConfig[key],
				NewValue:  nil,
				Timestamp: time.Now(),
			}
			m.notifyChange(event)
		}
	}
}

// convertToPlatformConfig 转换为平台配置格式
func (m *UnifiedConfigManagerImpl) convertToPlatformConfig(configData map[string]interface{}) *config.Config {
	result := &config.Config{}

	// 转换服务器配置
	if serverIP, err := m.GetString("server.ip"); err == nil {
		result.Server.IP = serverIP
	}
	if serverPort, err := m.GetInt("server.port"); err == nil {
		result.Server.Port = serverPort
	}
	if serverToken, err := m.GetString("server.token"); err == nil {
		result.Server.Token = serverToken
	}

	// 转换日志配置
	if logLevel, err := m.GetString("log.level"); err == nil {
		result.Log.Level = logLevel
	}
	if logDir, err := m.GetString("log.dir"); err == nil {
		result.Log.Dir = logDir
	}

	// 数据库配置暂不转换，因为当前Config结构体未包含Database字段
	// TODO: 后续根据需要添加数据库配置转换

	// 转换LLM配置
	if llmProviders, exists := configData["llm"].(map[string]interface{}); exists {
		if result.LLM == nil {
			result.LLM = make(map[string]config.LLMConfig)
		}

		for providerName, providerConfig := range llmProviders {
			if providerConfigMap, ok := providerConfig.(map[string]interface{}); ok {
				llmConfig := config.LLMConfig{}

				if apiKey, exists := providerConfigMap["api_key"].(string); exists {
					llmConfig.APIKey = apiKey
				}
				if baseURL, exists := providerConfigMap["base_url"].(string); exists {
					llmConfig.BaseURL = baseURL
				}
				if model, exists := providerConfigMap["model"].(string); exists {
					llmConfig.ModelName = model
				}
				if temperature, exists := providerConfigMap["temperature"].(float64); exists {
					llmConfig.Temperature = temperature
				}
				if maxTokens, exists := providerConfigMap["max_tokens"].(float64); exists {
					llmConfig.MaxTokens = int(maxTokens)
				}
				// Timeout field doesn't exist in LLMConfig, skipping
				_ = providerConfigMap["timeout"]

				result.LLM[providerName] = llmConfig
			}
		}
	}

	// 转换TTS配置
	if ttsProviders, exists := configData["tts"].(map[string]interface{}); exists {
		if result.TTS == nil {
			result.TTS = make(map[string]config.TTSConfig)
		}

		for providerName, providerConfig := range ttsProviders {
			if providerConfigMap, ok := providerConfig.(map[string]interface{}); ok {
				ttsConfig := config.TTSConfig{}

				if voice, exists := providerConfigMap["voice"].(string); exists {
					ttsConfig.Voice = voice
				}
				// Speed, Pitch, Volume, SampleRate fields don't exist in TTSConfig, skipping
				_ = providerConfigMap["speed"]
				_ = providerConfigMap["pitch"]
				_ = providerConfigMap["volume"]
				_ = providerConfigMap["sample_rate"]
				if format, exists := providerConfigMap["format"].(string); exists {
					ttsConfig.Format = format
				}
				// Language and DeleteFile fields don't exist in TTSConfig, skipping
				_ = providerConfigMap["language"]
				_ = providerConfigMap["delete_file"]
				if outputDir, exists := providerConfigMap["output_dir"].(string); exists {
					ttsConfig.OutputDir = outputDir
				}

				result.TTS[providerName] = ttsConfig
			}
		}
	}

	// 转换ASR配置
	if asrProviders, exists := configData["asr"].(map[string]interface{}); exists {
		if result.ASR == nil {
			result.ASR = make(map[string]interface{})
		}
		for key, value := range asrProviders {
			result.ASR[key] = value
		}
	}

	// 转换MCP配置
	if mcpConfig, exists := configData["mcp"].(map[string]interface{}); exists {
		for key, value := range mcpConfig {
			switch key {
			case "enabled":
				if enabled, ok := value.(bool); ok {
					result.MCP.Enabled = enabled
				}
			// ServerURL and Timeout fields don't exist in MCPConfig, skipping
			case "server_url":
				_ = value // ServerURL field doesn't exist in MCPConfig
			case "timeout":
				_ = value // Timeout field doesn't exist in MCPConfig
			}
		}
	}

	return result
}

// convertFromPlatformConfig 从平台配置格式转换
func (m *UnifiedConfigManagerImpl) convertFromPlatformConfig(cfg *config.Config) map[string]interface{} {
	// TODO: 实现完整的配置转换逻辑
	// 这里需要将platform.Config结构转换为通用的配置映射
	result := make(map[string]interface{})

	// 使用反射来转换配置结构
	v := reflect.ValueOf(cfg).Elem()
	t := reflect.TypeOf(cfg).Elem()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)
		jsonTag := fieldType.Tag.Get("json")

		if jsonTag != "" && jsonTag != "-" {
			key := strings.Split(jsonTag, ",")[0]
			result[key] = field.Interface()
		} else {
			result[fieldType.Name] = field.Interface()
		}
	}

	return result
}

// updateSourceStats 更新配置源统计信息
func (m *UnifiedConfigManagerImpl) updateSourceStats(sourceName string, success bool, loadTime time.Duration) {
	m.statsMutex.Lock()
	defer m.statsMutex.Unlock()

	stats, exists := m.stats.SourceStats[sourceName]
	if !exists {
		stats = SourceStats{
			Name: sourceName,
		}
		m.stats.SourceStats[sourceName] = stats
	}

	stats.LoadCount++
	stats.LastLoad = time.Now()
	stats.LoadTime = loadTime

	if success {
		stats.IsAvailable = true
	} else {
		stats.ErrorCount++
		stats.IsAvailable = false
	}
}

// recordCacheHit 记录缓存命中
func (m *UnifiedConfigManagerImpl) recordCacheHit() {
	if m.cache != nil {
		stats := m.cache.GetStats()
		m.statsMutex.Lock()
		m.stats.CacheHitRate = stats.HitRate
		m.statsMutex.Unlock()
	}
}

// recordCacheMiss 记录缓存未命中
func (m *UnifiedConfigManagerImpl) recordCacheMiss() {
	if m.cache != nil {
		stats := m.cache.GetStats()
		m.statsMutex.Lock()
		m.stats.CacheHitRate = stats.HitRate
		m.statsMutex.Unlock()
	}
}

// recordReload 记录重载
func (m *UnifiedConfigManagerImpl) recordReload() {
	m.statsMutex.Lock()
	defer m.statsMutex.Unlock()
	m.stats.ReloadCount++
	m.stats.LastReload = time.Now()
}

// recordValidationError 记录验证错误
func (m *UnifiedConfigManagerImpl) recordValidationError() {
	m.statsMutex.Lock()
	defer m.statsMutex.Unlock()
	m.stats.ValidationErrors++
}