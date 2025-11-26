package config

import (
	"context"
	"time"

	"xiaozhi-server-go/internal/platform/config"
)

// ConfigSource 配置源接口
// 定义了从不同来源获取配置的能力
type ConfigSource interface {
	// GetName 获取配置源名称
	GetName() string

	// GetPriority 获取配置源优先级（数字越大优先级越高）
	GetPriority() int

	// Load 加载配置数据
	Load(ctx context.Context) (map[string]interface{}, error)

	// Watch 监听配置变化（可选实现）
	Watch(ctx context.Context) (<-chan ConfigChangeEvent, error)

	// IsAvailable 检查配置源是否可用
	IsAvailable(ctx context.Context) bool

	// Close 关闭配置源
	Close() error
}

// ConfigChangeEvent 配置变更事件
type ConfigChangeEvent struct {
	Source    string                 `json:"source"`
	Key       string                 `json:"key"`
	OldValue  interface{}            `json:"old_value,omitempty"`
	NewValue  interface{}            `json:"new_value"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ConfigValidator 配置验证器接口
type ConfigValidator interface {
	// ValidateSchema 验证配置结构是否符合Schema
	ValidateSchema(config map[string]interface{}) error

	// ValidateValue 验证具体配置值
	ValidateValue(key string, value interface{}) error

	// GetSchema 获取配置Schema
	GetSchema() map[string]interface{}

	// AddCustomRule 添加自定义验证规则
	AddCustomRule(key string, rule ValidationRule) error

	// RemoveCustomRule 移除自定义验证规则
	RemoveCustomRule(key string) error
}

// ValidationRule 验证规则
type ValidationRule struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Validator   func(interface{}) error `json:"-"`
	Required    bool        `json:"required"`
}

// ConfigCache 配置缓存接口
type ConfigCache interface {
	// Get 获取缓存的配置值
	Get(key string) (interface{}, bool)

	// Set 设置配置值到缓存
	Set(key string, value interface{}, ttl time.Duration) error

	// Delete 删除缓存项
	Delete(key string) error

	// Clear 清空缓存
	Clear() error

	// GetStats 获取缓存统计信息
	GetStats() CacheStats

	// SetTTL 设置默认TTL
	SetTTL(ttl time.Duration)

	// StartGC 启动垃圾回收
	StartGC(interval time.Duration)

	// StopGC 停止垃圾回收
	StopGC()
}

// CacheStats 缓存统计信息
type CacheStats struct {
	Hits        int64   `json:"hits"`
	Misses      int64   `json:"misses"`
	Size        int64   `json:"size"`
	HitRate     float64 `json:"hit_rate"`
	Evictions   int64   `json:"evictions"`
	LastGC      time.Time `json:"last_gc,omitempty"`
}

// ConfigNotifier 配置变更通知器接口
type ConfigNotifier interface {
	// Subscribe 订阅配置变更
	Subscribe(pattern string, subscriber ConfigChangeSubscriber) error

	// Unsubscribe 取消订阅
	Unsubscribe(pattern string, subscriber ConfigChangeSubscriber) error

	// Notify 通知配置变更
	Notify(event ConfigChangeEvent) error

	// GetSubscribers 获取订阅者列表
	GetSubscribers() map[string][]ConfigChangeSubscriber
}

// ConfigChangeSubscriber 配置变更订阅者接口
type ConfigChangeSubscriber interface {
	// GetID 获取订阅者ID
	GetID() string

	// OnConfigChange 处理配置变更事件
	OnConfigChange(ctx context.Context, event ConfigChangeEvent) error

	// GetFilter 获取变更过滤器
	GetFilter() string

	// IsAsync 是否异步处理
	IsAsync() bool
}

// UnifiedConfigManager 统一配置管理器接口
// 核心配置管理功能，聚合所有配置源并提供统一的访问接口
type UnifiedConfigManager interface {
	// Initialize 初始化配置管理器
	Initialize(ctx context.Context) error

	// Cleanup 清理资源
	Cleanup() error

	// Get 获取配置值
	Get(key string) (interface{}, error)

	// GetWithDefault 获取配置值，带默认值
	GetWithDefault(key string, defaultValue interface{}) interface{}

	// GetString 获取字符串配置值
 GetString(key string) (string, error)

	// GetStringWithDefault 获取字符串配置值，带默认值
	GetStringWithDefault(key, defaultValue string) string

	// GetInt 获取整数配置值
	GetInt(key string) (int, error)

	// GetIntWithDefault 获取整数配置值，带默认值
	GetIntWithDefault(key string, defaultValue int) int

	// GetBool 获取布尔配置值
	GetBool(key string) (bool, error)

	// GetBoolWithDefault 获取布尔配置值，带默认值
	GetBoolWithDefault(key string, defaultValue bool) bool

	// GetDuration 获取时间间隔配置值
	GetDuration(key string) (time.Duration, error)

	// GetDurationWithDefault 获取时间间隔配置值，带默认值
	GetDurationWithDefault(key string, defaultValue time.Duration) time.Duration

	// Set 设置配置值
	Set(key string, value interface{}) error

	// SetWithSource 设置配置值并指定来源
	SetWithSource(key string, value interface{}, source string) error

	// Delete 删除配置项
	Delete(key string) error

	// GetAll 获取所有配置
	GetAll() (map[string]interface{}, error)

	// Reload 重新加载配置
	Reload(ctx context.Context) error

	// Save 保存配置到持久化存储
	Save(ctx context.Context) error

	// AddSource 添加配置源
	AddSource(source ConfigSource) error

	// RemoveSource 移除配置源
	RemoveSource(sourceName string) error

	// GetSources 获取所有配置源
	GetSources() []ConfigSource

	// SetValidator 设置配置验证器
	SetValidator(validator ConfigValidator)

	// SetCache 设置配置缓存
	SetCache(cache ConfigCache)

	// SetNotifier 设置变更通知器
	SetNotifier(notifier ConfigNotifier)

	// GetStats 获取统计信息
	GetStats() UnifiedConfigStats

	// HealthCheck 健康检查
	HealthCheck(ctx context.Context) error

	// Export 导出配置
	Export(ctx context.Context) (*config.Config, error)

	// Import 导入配置
	Import(ctx context.Context, cfg *config.Config) error
}

// UnifiedConfigStats 统一配置管理器统计信息
type UnifiedConfigStats struct {
	TotalSources     int                    `json:"total_sources"`
	ActiveSources    int                    `json:"active_sources"`
	TotalKeys        int                    `json:"total_keys"`
	CacheHitRate     float64                `json:"cache_hit_rate"`
	LastReload       time.Time              `json:"last_reload"`
	ReloadCount      int64                  `json:"reload_count"`
	ValidationErrors int64                  `json:"validation_errors"`
	SourceStats      map[string]SourceStats `json:"source_stats"`
	CacheStats       CacheStats             `json:"cache_stats"`
}

// SourceStats 配置源统计信息
type SourceStats struct {
	Name       string        `json:"name"`
	Priority   int           `json:"priority"`
	IsAvailable bool          `json:"is_available"`
	LastLoad   time.Time     `json:"last_load"`
	LoadCount  int64         `json:"load_count"`
	ErrorCount int64         `json:"error_count"`
	LoadTime   time.Duration `json:"load_time"`
}

// ConfigMergeStrategy 配置合并策略
type ConfigMergeStrategy int

const (
	// MergeStrategyOverwrite 覆盖策略：高优先级覆盖低优先级
	MergeStrategyOverwrite ConfigMergeStrategy = iota

	// MergeStrategyMerge 合并策略：智能合并配置
	MergeStrategyMerge

	// MergeStrategyAppend 追加策略：追加而不覆盖
	MergeStrategyAppend
)

// ConfigHotReloadMode 配置热重载模式
type ConfigHotReloadMode int

const (
	// HotReloadDisabled 禁用热重载
	HotReloadDisabled ConfigHotReloadMode = iota

	// HotReloadAuto 自动热重载
	HotReloadAuto

	// HotReloadManual 手动热重载
	HotReloadManual
)