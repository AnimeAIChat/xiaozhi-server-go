package factory

import (
	"fmt"
	"sync"

	contractProviders "xiaozhi-server-go/internal/contracts/providers"
	"xiaozhi-server-go/internal/contracts/manager"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/src/core/utils"
)

// ProviderFactory 提供者工厂接口
type ProviderFactory interface {
	// CreateASRProvider 创建ASR提供者
	CreateASRProvider(providerType string, cfg *config.Config, logger *utils.Logger) (contractProviders.ASRProvider, error)

	// CreateLLMProvider 创建LLM提供者
	CreateLLMProvider(providerType string, cfg *config.Config, logger *utils.Logger) (contractProviders.LLMProvider, error)

	// CreateTTSProvider 创建TTS提供者
	CreateTTSProvider(providerType string, cfg *config.Config, logger *utils.Logger) (contractProviders.TTSProvider, error)

	// GetSupportedProviders 获取支持的提供者类型
	GetSupportedProviders() map[string][]string

	// ValidateProviderConfig 验证提供者配置
	ValidateProviderConfig(providerType, category string, cfg *config.Config) error
}

// DefaultProviderFactory 默认提供者工厂实现
type DefaultProviderFactory struct {
	registry contractProviders.ProviderRegistry
	config   *config.Config
	logger   *utils.Logger
}

// NewDefaultProviderFactory 创建默认提供者工厂
func NewDefaultProviderFactory(cfg *config.Config, logger *utils.Logger) ProviderFactory {
	factory := &DefaultProviderFactory{
		registry: contractProviders.NewDefaultProviderRegistry(),
		config:   cfg,
		logger:   logger,
	}

	// 初始化并注册所有内置提供者
	factory.registerBuiltinProviders()

	return factory
}

// registerBuiltinProviders 注册内置提供者
func (f *DefaultProviderFactory) registerBuiltinProviders() {
	// 注意：这里暂时注释掉具体的提供者注册，避免循环依赖
	// 在后续阶段我们会逐步迁移现有的提供者实现

	// 示例注册模式（将在后续实现）：
	// f.registerASRProvider("openai", &OpenAIWhisperFactory{})
	// f.registerLLMProvider("openai", &OpenAIGPTFactory{})
	// f.registerTTSProvider("edge", &EdgeTTSFactory{})
}

// CreateASRProvider 创建ASR提供者
func (f *DefaultProviderFactory) CreateASRProvider(providerType string, cfg *config.Config, logger *utils.Logger) (contractProviders.ASRProvider, error) {
	if f == nil {
		return nil, fmt.Errorf("provider factory not initialized")
	}

	// 使用统一提供者管理器
	providerManager := manager.NewUnifiedProviderManager(cfg, logger)
	if err := providerManager.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize provider manager: %w", err)
	}

	return providerManager.CreateASRProvider(providerType, cfg, map[string]interface{}{
		"logger": logger,
	})
}

// CreateLLMProvider 创建LLM提供者
func (f *DefaultProviderFactory) CreateLLMProvider(providerType string, cfg *config.Config, logger *utils.Logger) (contractProviders.LLMProvider, error) {
	if f == nil {
		return nil, fmt.Errorf("provider factory not initialized")
	}

	// 使用统一提供者管理器
	providerManager := manager.NewUnifiedProviderManager(cfg, logger)
	if err := providerManager.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize provider manager: %w", err)
	}

	return providerManager.CreateLLMProvider(providerType, cfg, map[string]interface{}{
		"logger": logger,
	})
}

// CreateTTSProvider 创建TTS提供者
func (f *DefaultProviderFactory) CreateTTSProvider(providerType string, cfg *config.Config, logger *utils.Logger) (contractProviders.TTSProvider, error) {
	if f == nil {
		return nil, fmt.Errorf("provider factory not initialized")
	}

	// 使用统一提供者管理器
	providerManager := manager.NewUnifiedProviderManager(cfg, logger)
	if err := providerManager.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize provider manager: %w", err)
	}

	return providerManager.CreateTTSProvider(providerType, cfg, map[string]interface{}{
		"logger": logger,
	})
}

// GetSupportedProviders 获取支持的提供者类型
func (f *DefaultProviderFactory) GetSupportedProviders() map[string][]string {
	// 创建临时管理器来获取支持的提供者
	if f.config == nil || f.logger == nil {
		return map[string][]string{
			"asr": {"doubao"}, // 基本支持
			"llm": {"openai"},
			"tts": {"edge"},
		}
	}

	tempManager := manager.NewUnifiedProviderManager(f.config, f.logger)
	return tempManager.GetAvailableProviders()
}

// ValidateProviderConfig 验证提供者配置
func (f *DefaultProviderFactory) ValidateProviderConfig(providerType, category string, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// TODO: 实现配置验证逻辑
	// 需要根据不同的providerType和category进行相应的配置验证
	return nil
}

// ProviderFactoryHolder 提供者工厂持有者，用于单例管理
type ProviderFactoryHolder struct {
	factory ProviderFactory
}

var (
	holder *ProviderFactoryHolder
	once   sync.Once
)

// GetProviderFactory 获取全局提供者工厂（单例模式）
func GetProviderFactory(cfg *config.Config, logger *utils.Logger) ProviderFactory {
	once.Do(func() {
		holder = &ProviderFactoryHolder{
			factory: NewDefaultProviderFactory(cfg, logger),
		}
	})
	return holder.factory
}

// ResetProviderFactory 重置全局提供者工厂（主要用于测试）
func ResetProviderFactory() {
	once = sync.Once{}
	holder = nil
}