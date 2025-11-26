package adapters

import (
	"context"
	"fmt"

	contractProviders "xiaozhi-server-go/internal/contracts/providers"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/src/core/utils"
)

// LegacyPoolManagerAdapter 旧版资源池管理器适配器
// 这个适配器用于在过渡期间保持与旧代码的兼容性
type LegacyPoolManagerAdapter struct {
	// 使用接口而不是具体实现，避免循环依赖
	providerFactory ProviderFactoryInterface
	logger          *utils.Logger
}

// ProviderFactoryInterface 提供者工厂接口（避免循环依赖）
type ProviderFactoryInterface interface {
	CreateASRProvider(providerType string, cfg *config.Config, logger *utils.Logger) (contractProviders.ASRProvider, error)
	CreateLLMProvider(providerType string, cfg *config.Config, logger *utils.Logger) (contractProviders.LLMProvider, error)
	CreateTTSProvider(providerType string, cfg *config.Config, logger *utils.Logger) (contractProviders.TTSProvider, error)
}

// NewLegacyPoolManagerAdapter 创建旧版资源池管理器适配器
func NewLegacyPoolManagerAdapter(factory ProviderFactoryInterface, logger *utils.Logger) *LegacyPoolManagerAdapter {
	return &LegacyPoolManagerAdapter{
		providerFactory: factory,
		logger:          logger,
	}
}

// GetProviderSet 获取提供者集合（兼容旧接口）
func (a *LegacyPoolManagerAdapter) GetProviderSet() (interface{}, error) {
	// TODO: 实现提供者集合获取逻辑
	// 这里需要返回一个兼容旧ProviderSet接口的对象
	return nil, fmt.Errorf("GetProviderSet not yet implemented in adapter")
}

// ReturnProviderSet 返回提供者集合（兼容旧接口）
func (a *LegacyPoolManagerAdapter) ReturnProviderSet(set interface{}) error {
	// TODO: 实现提供者集合返回逻辑
	return nil
}

// Warmup 预热资源池（兼容旧接口）
func (a *LegacyPoolManagerAdapter) Warmup(ctx context.Context) error {
	if a.logger != nil {
		a.logger.InfoTag("适配器", "资源池预热开始")
	}

	// TODO: 实现预热逻辑
	// 预热所有常用的提供者实例以减少首次请求延迟

	if a.logger != nil {
		a.logger.InfoTag("适配器", "资源池预热完成")
	}
	return nil
}

// GetStats 获取统计信息（兼容旧接口）
func (a *LegacyPoolManagerAdapter) GetStats() map[string]map[string]int {
	// 返回空统计信息，等待后续实现
	return map[string]map[string]int{
		"asr": {"available": 0, "in_use": 0, "total": 0},
		"llm": {"available": 0, "in_use": 0, "total": 0},
		"tts": {"available": 0, "in_use": 0, "total": 0},
	}
}

// Close 关闭适配器（兼容旧接口）
func (a *LegacyPoolManagerAdapter) Close() {
	if a.logger != nil {
		a.logger.InfoTag("适配器", "LegacyPoolManagerAdapter 正在关闭")
	}
	// TODO: 实现资源清理逻辑
}

// BootstrapManager 引导管理器，负责在不产生循环依赖的情况下初始化组件
type BootstrapManager struct {
	config *config.Config
	logger *utils.Logger
}

// NewBootstrapManager 创建引导管理器
func NewBootstrapManager(cfg *config.Config, logger *utils.Logger) *BootstrapManager {
	return &BootstrapManager{
		config: cfg,
		logger: logger,
	}
}

// InitializeComponents 初始化组件（无循环依赖版本）
func (bm *BootstrapManager) InitializeComponents() (*ComponentContainer, error) {
	if bm.logger != nil {
		bm.logger.InfoTag("引导管理器", "开始初始化组件...")
	}

	container := &ComponentContainer{
		config: bm.config,
		logger: bm.logger,
	}

	// 初始化适配器，避免直接导入src/core
	providerFactory := NewSafeProviderFactory(bm.config, bm.logger)
	container.legacyAdapter = NewLegacyPoolManagerAdapter(providerFactory, bm.logger)

	if bm.logger != nil {
		bm.logger.InfoTag("引导管理器", "组件初始化完成")
	}

	return container, nil
}

// ComponentContainer 组件容器，持有所有初始化的组件
type ComponentContainer struct {
	config         *config.Config
	logger         *utils.Logger
	legacyAdapter  *LegacyPoolManagerAdapter
	providerFactory ProviderFactoryInterface
}

// GetLegacyAdapter 获取旧版适配器
func (c *ComponentContainer) GetLegacyAdapter() *LegacyPoolManagerAdapter {
	return c.legacyAdapter
}

// GetProviderFactory 获取提供者工厂
func (c *ComponentContainer) GetProviderFactory() ProviderFactoryInterface {
	return c.providerFactory
}

// SafeProviderFactory 安全的提供者工厂实现，避免循环依赖
type SafeProviderFactory struct {
	config *config.Config
	logger *utils.Logger
}

// NewSafeProviderFactory 创建安全的提供者工厂
func NewSafeProviderFactory(cfg *config.Config, logger *utils.Logger) *SafeProviderFactory {
	return &SafeProviderFactory{
		config: cfg,
		logger: logger,
	}
}

// CreateASRProvider 创建ASR提供者
func (f *SafeProviderFactory) CreateASRProvider(providerType string, cfg *config.Config, logger *utils.Logger) (contractProviders.ASRProvider, error) {
	// TODO: 在第二阶段实现具体的ASR提供者创建逻辑
	// 现在先返回占位符实现
	if logger != nil {
		logger.InfoTag("安全工厂", "创建ASR提供者: %s", providerType)
	}
	return nil, fmt.Errorf("ASR provider '%s' will be implemented in phase 2", providerType)
}

// CreateLLMProvider 创建LLM提供者
func (f *SafeProviderFactory) CreateLLMProvider(providerType string, cfg *config.Config, logger *utils.Logger) (contractProviders.LLMProvider, error) {
	// TODO: 在第二阶段实现具体的LLM提供者创建逻辑
	// 现在先返回占位符实现
	if logger != nil {
		logger.InfoTag("安全工厂", "创建LLM提供者: %s", providerType)
	}
	return nil, fmt.Errorf("LLM provider '%s' will be implemented in phase 2", providerType)
}

// CreateTTSProvider 创建TTS提供者
func (f *SafeProviderFactory) CreateTTSProvider(providerType string, cfg *config.Config, logger *utils.Logger) (contractProviders.TTSProvider, error) {
	// TODO: 在第二阶段实现具体的TTS提供者创建逻辑
	// 现在先返回占位符实现
	if logger != nil {
		logger.InfoTag("安全工厂", "创建TTS提供者: %s", providerType)
	}
	return nil, fmt.Errorf("TTS provider '%s' will be implemented in phase 2", providerType)
}