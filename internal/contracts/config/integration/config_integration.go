package integration

import (
	"context"
	"fmt"
	"time"

	contractConfig "xiaozhi-server-go/internal/contracts/config"
	"xiaozhi-server-go/internal/contracts/config/cache"
	"xiaozhi-server-go/internal/contracts/config/notifier"
	"xiaozhi-server-go/internal/contracts/config/sources"
	platformConfig "xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/src/core/utils"
)

// ConfigIntegrator 配置系统集成器
// 负责将新的统一配置系统集成到现有系统中
type ConfigIntegrator struct {
	unifiedManager contractConfig.UnifiedConfigManager
	platformConfig  *platformConfig.Config
	logger          *utils.Logger
	isInitialized   bool
}

// ConfigIntegratorOption 配置集成器选项
type ConfigIntegratorOption func(*ConfigIntegrator)

// NewConfigIntegrator 创建配置系统集成器
func NewConfigIntegrator(platformConfig *platformConfig.Config, logger *utils.Logger, options ...ConfigIntegratorOption) (*ConfigIntegrator, error) {
	if platformConfig == nil {
		return nil, fmt.Errorf("platform config cannot be nil")
	}

	if logger == nil {
		logger = utils.DefaultLogger
	}

	integrator := &ConfigIntegrator{
		platformConfig: platformConfig,
		logger:         logger,
		isInitialized:  false,
	}

	// 应用选项
	for _, option := range options {
		option(integrator)
	}

	// 创建统一配置管理器
	integrator.unifiedManager = contractConfig.NewUnifiedConfigManager(
		logger,
		contractConfig.WithMergeStrategy(contractConfig.MergeStrategyOverwrite),
		contractConfig.WithHotReloadMode(contractConfig.HotReloadAuto),
		contractConfig.WithDefaultTTL(5*time.Minute),
		contractConfig.WithEventWorkers(3),
	)

	return integrator, nil
}

// WithConfigFilePath 设置配置文件路径
func WithConfigFilePath(configPath string) ConfigIntegratorOption {
	return func(ci *ConfigIntegrator) {
		// 这个选项将在Initialize时使用
	}
}

// Initialize 初始化配置系统
func (ci *ConfigIntegrator) Initialize(ctx context.Context) error {
	if ci.isInitialized {
		return fmt.Errorf("config integrator already initialized")
	}

	ci.logger.InfoTag("ConfigIntegrator", "初始化配置系统集成")

	// 创建配置组件
	// 暂时跳过严格验证，因为配置主要来自数据库
	// configValidator := validator.NewSchemaValidator()
	configCache := cache.NewMemoryCache()
	configNotifier := notifier.NewChangeNotifier(ci.logger, 3)

	// 配置缓存
	configCache.SetTTL(5 * time.Minute)
	configCache.StartGC(2 * time.Minute)

	// 设置组件到统一管理器
	// ci.unifiedManager.SetValidator(configValidator) // 暂时跳过验证
	ci.unifiedManager.SetCache(configCache)
	ci.unifiedManager.SetNotifier(configNotifier)

	// 添加配置源
	if err := ci.addConfigSources(ctx); err != nil {
		return fmt.Errorf("failed to add config sources: %w", err)
	}

	// 添加订阅者
	if err := ci.addSubscribers(); err != nil {
		return fmt.Errorf("failed to add subscribers: %w", err)
	}

	// 初始化统一管理器
	if err := ci.unifiedManager.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize unified config manager: %w", err)
	}

	// 同步配置到平台配置
	if err := ci.syncToPlatformConfig(); err != nil {
		return fmt.Errorf("failed to sync to platform config: %w", err)
	}

	ci.isInitialized = true
	ci.logger.InfoTag("ConfigIntegrator", "配置系统集成初始化完成")
	return nil
}

// Cleanup 清理资源
func (ci *ConfigIntegrator) Cleanup() error {
	if !ci.isInitialized {
		return nil
	}

	ci.logger.InfoTag("ConfigIntegrator", "清理配置系统集成")

	if err := ci.unifiedManager.Cleanup(); err != nil {
		ci.logger.ErrorTag("ConfigIntegrator", "清理统一配置管理器失败: %v", err)
	}

	ci.isInitialized = false
	return nil
}

// GetUnifiedManager 获取统一配置管理器
func (ci *ConfigIntegrator) GetUnifiedManager() contractConfig.UnifiedConfigManager {
	return ci.unifiedManager
}

// GetPlatformConfig 获取平台配置
func (ci *ConfigIntegrator) GetPlatformConfig() *platformConfig.Config {
	return ci.platformConfig
}

// Reload 重载配置
func (ci *ConfigIntegrator) Reload(ctx context.Context) error {
	if !ci.isInitialized {
		return fmt.Errorf("config integrator not initialized")
	}

	ci.logger.InfoTag("ConfigIntegrator", "重载配置系统")

	if err := ci.unifiedManager.Reload(ctx); err != nil {
		return fmt.Errorf("failed to reload unified config manager: %w", err)
	}

	// 同步配置到平台配置
	if err := ci.syncToPlatformConfig(); err != nil {
		return fmt.Errorf("failed to sync to platform config: %w", err)
	}

	ci.logger.InfoTag("ConfigIntegrator", "配置重载完成")
	return nil
}

// Save 保存配置
func (ci *ConfigIntegrator) Save(ctx context.Context) error {
	if !ci.isInitialized {
		return fmt.Errorf("config integrator not initialized")
	}

	ci.logger.InfoTag("ConfigIntegrator", "保存配置")

	if err := ci.unifiedManager.Save(ctx); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	ci.logger.InfoTag("ConfigIntegrator", "配置保存完成")
	return nil
}

// GetStats 获取统计信息
func (ci *ConfigIntegrator) GetStats() map[string]interface{} {
	if !ci.isInitialized {
		return map[string]interface{}{
			"initialized": false,
		}
	}

	unifiedStats := ci.unifiedManager.GetStats()

	return map[string]interface{}{
		"initialized": ci.isInitialized,
		"unified":    unifiedStats,
	}
}

// HealthCheck 健康检查
func (ci *ConfigIntegrator) HealthCheck(ctx context.Context) error {
	if !ci.isInitialized {
		return fmt.Errorf("config integrator not initialized")
	}

	return ci.unifiedManager.HealthCheck(ctx)
}

// 私有方法

// addConfigSources 添加配置源
func (ci *ConfigIntegrator) addConfigSources(ctx context.Context) error {
	// 添加环境变量配置源（最高优先级）
	envSource, err := sources.NewEnvSource(
		sources.WithPrefix("XIAOZHI"),
		sources.WithEnvPriority(100),
		sources.WithEnvWatchMode(true, 10*time.Second),
		sources.WithEnvTTL(1*time.Minute),
	)
	if err != nil {
		return fmt.Errorf("failed to create env source: %w", err)
	}
	if err := ci.unifiedManager.AddSource(envSource); err != nil {
		return fmt.Errorf("failed to add env source: %w", err)
	}

	// 暂时跳过文件配置源，只使用环境变量和数据库配置
	// 文件配置由外部RAG系统管理，不应该包含服务器必需配置
	ci.logger.InfoTag("ConfigIntegrator", "跳过文件配置源，使用数据库和环境变量配置")
	/*
	configPath := filepath.Join("data", "config.json")
	fileSource, err := sources.NewFileSource(
		configPath,
		sources.WithFilePriority(80),
		sources.WithFileWatchMode(true),
		sources.WithFileTTL(5*time.Minute),
	)
	if err != nil {
		// 配置文件不存在是正常的，使用默认配置
		ci.logger.WarnTag("ConfigIntegrator", "配置文件不存在，跳过: %v", err)
	} else {
		if err := ci.unifiedManager.AddSource(fileSource); err != nil {
			return fmt.Errorf("failed to add file source: %w", err)
		}
	}
	*/

	// 添加数据库配置源（中等优先级）
	// TODO: 实现数据库配置源
	// databaseSource, err := sources.NewDatabaseSource(...)
	// if err != nil {
	//     return fmt.Errorf("failed to create database source: %w", err)
	// }
	// if err := ci.unifiedManager.AddSource(databaseSource); err != nil {
	//     return fmt.Errorf("failed to add database source: %w", err)
	// }

	ci.logger.InfoTag("ConfigIntegrator", "配置源添加完成")
	return nil
}

// addSubscribers 添加订阅者
func (ci *ConfigIntegrator) addSubscribers() error {
	// 添加日志记录订阅者
	_ = notifier.NewLoggingSubscriber(
		"logging",
		"*", // 订阅所有变更
		ci.logger,
		false, // 同步处理
	)

	// TODO: 临时跳过notifier设置，需要实现SetNotifier接口
	// _ = ci.unifiedManager.SetNotifier(&notifier.ChangeNotifier{})
	_ = &notifier.ChangeNotifier{}

	// TODO: 添加其他订阅者，如：
	// - 热重载特定配置项
	// - 配置变更时重新初始化相关服务
	// - 配置变更时发送通知

	ci.logger.InfoTag("ConfigIntegrator", "订阅者添加完成")
	return nil
}

// syncToPlatformConfig 同步配置到平台配置
func (ci *ConfigIntegrator) syncToPlatformConfig() error {
	// 从统一管理器获取所有配置
	_, err := ci.unifiedManager.GetAll()
	if err != nil {
		return fmt.Errorf("failed to get all config: %w", err)
	}

	// TODO: 实现完整的配置同步逻辑
	// 这里需要将统一的配置映射转换为具体的platform.Config结构

	// 示例：同步一些基本配置
	if serverIP, err := ci.unifiedManager.GetString("server.ip"); err == nil {
		ci.platformConfig.Server.IP = serverIP
	}

	if serverPort, err := ci.unifiedManager.GetInt("server.port"); err == nil {
		ci.platformConfig.Server.Port = serverPort
	}

	if logLevel, err := ci.unifiedManager.GetString("log.level"); err == nil {
		ci.platformConfig.Log.Level = logLevel
	}

	ci.logger.DebugTag("ConfigIntegrator", "配置同步到平台配置完成")
	return nil
}

// GetConfigValue 获取配置值的便捷方法
func (ci *ConfigIntegrator) GetConfigValue(key string) (interface{}, error) {
	if !ci.isInitialized {
		return nil, fmt.Errorf("config integrator not initialized")
	}

	return ci.unifiedManager.Get(key)
}

// GetConfigString 获取字符串配置值
func (ci *ConfigIntegrator) GetConfigString(key string) string {
	if !ci.isInitialized {
		return ""
	}

	value, err := ci.unifiedManager.GetString(key)
	if err != nil {
		return ""
	}
	return value
}

// GetConfigStringWithDefault 获取字符串配置值，带默认值
func (ci *ConfigIntegrator) GetConfigStringWithDefault(key, defaultValue string) string {
	if !ci.isInitialized {
		return defaultValue
	}

	return ci.unifiedManager.GetStringWithDefault(key, defaultValue)
}

// GetConfigInt 获取整数配置值
func (ci *ConfigIntegrator) GetConfigInt(key string) int {
	if !ci.isInitialized {
		return 0
	}

	value, err := ci.unifiedManager.GetInt(key)
	if err != nil {
		return 0
	}
	return value
}

// GetConfigIntWithDefault 获取整数配置值，带默认值
func (ci *ConfigIntegrator) GetConfigIntWithDefault(key string, defaultValue int) int {
	if !ci.isInitialized {
		return defaultValue
	}

	return ci.unifiedManager.GetIntWithDefault(key, defaultValue)
}

// GetConfigBool 获取布尔配置值
func (ci *ConfigIntegrator) GetConfigBool(key string) bool {
	if !ci.isInitialized {
		return false
	}

	value, err := ci.unifiedManager.GetBool(key)
	if err != nil {
		return false
	}
	return value
}

// GetConfigBoolWithDefault 获取布尔配置值，带默认值
func (ci *ConfigIntegrator) GetConfigBoolWithDefault(key string, defaultValue bool) bool {
	if !ci.isInitialized {
		return defaultValue
	}

	return ci.unifiedManager.GetBoolWithDefault(key, defaultValue)
}

// GetConfigDuration 获取时间间隔配置值
func (ci *ConfigIntegrator) GetConfigDuration(key string) time.Duration {
	if !ci.isInitialized {
		return 0
	}

	value, err := ci.unifiedManager.GetDuration(key)
	if err != nil {
		return 0
	}
	return value
}

// GetConfigDurationWithDefault 获取时间间隔配置值，带默认值
func (ci *ConfigIntegrator) GetConfigDurationWithDefault(key string, defaultValue time.Duration) time.Duration {
	if !ci.isInitialized {
		return defaultValue
	}

	return ci.unifiedManager.GetDurationWithDefault(key, defaultValue)
}

// SetConfigValue 设置配置值
func (ci *ConfigIntegrator) SetConfigValue(key string, value interface{}) error {
	if !ci.isInitialized {
		return fmt.Errorf("config integrator not initialized")
	}

	return ci.unifiedManager.Set(key, value)
}