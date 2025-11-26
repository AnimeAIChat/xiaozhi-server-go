package providers

import (
	"context"
)

// ProviderRegistry 提供者注册表接口
type ProviderRegistry interface {
	// RegisterASRProvider 注册ASR提供者
	RegisterASRProvider(name string, factory ASRProviderFactory) error

	// RegisterLLMProvider 注册LLM提供者
	RegisterLLMProvider(name string, factory LLMProviderFactory) error

	// RegisterTTSProvider 注册TTS提供者
	RegisterTTSProvider(name string, factory TTSProviderFactory) error

	// GetASRProvider 获取ASR提供者
	GetASRProvider(name string) (ASRProvider, error)

	// GetLLMProvider 获取LLM提供者
	GetLLMProvider(name string) (LLMProvider, error)

	// GetTTSProvider 获取TTS提供者
	GetTTSProvider(name string) (TTSProvider, error)

	// ListProviders 列出所有已注册的提供者
	ListProviders() map[string][]string
}

// ASRProviderFactory ASR提供者工厂接口
type ASRProviderFactory interface {
	// CreateProvider 创建ASR提供者实例
	CreateProvider(config interface{}, options map[string]interface{}) (ASRProvider, error)

	// GetProviderName 获取提供者名称
	GetProviderName() string

	// ValidateConfig 验证配置
	ValidateConfig(config interface{}) error
}

// LLMProviderFactory LLM提供者工厂接口
type LLMProviderFactory interface {
	// CreateProvider 创建LLM提供者实例
	CreateProvider(config interface{}, options map[string]interface{}) (LLMProvider, error)

	// GetProviderName 获取提供者名称
	GetProviderName() string

	// ValidateConfig 验证配置
	ValidateConfig(config interface{}) error
}

// TTSProviderFactory TTS提供者工厂接口
type TTSProviderFactory interface {
	// CreateProvider 创建TTS提供者实例
	CreateProvider(config interface{}, options map[string]interface{}) (TTSProvider, error)

	// GetProviderName 获取提供者名称
	GetProviderName() string

	// ValidateConfig 验证配置
	ValidateConfig(config interface{}) error
}

// ProviderManager 提供者管理器接口
type ProviderManager interface {
	// Initialize 初始化管理器
	Initialize() error

	// Shutdown 关闭管理器
	Shutdown() error

	// HealthCheck 健康检查所有提供者
	HealthCheck(ctx context.Context) map[string]error

	// GetStats 获取统计信息
	GetStats() ProviderStats
}

// ProviderStats 提供者统计信息
type ProviderStats struct {
	// TotalProviders 总提供者数量
	TotalProviders int `json:"total_providers"`

	// ActiveProviders 活跃提供者数量
	ActiveProviders int `json:"active_providers"`

	// ASRStats ASR统计
	ASRStats map[string]int `json:"asr_stats"`

	// LLMStats LLM统计
	LLMStats map[string]int `json:"llm_stats"`

	// TTSStats TTS统计
	TTSStats map[string]int `json:"tts_stats"`
}

// ProviderConfig 提供者配置接口
type ProviderConfig interface {
	// GetType 获取提供者类型
	GetType() string

	// GetName 获取提供者名称
	GetName() string

	// GetConfig 获取配置数据
	GetConfig() map[string]interface{}

	// Validate 验证配置
	Validate() error

	// Clone 克隆配置
	Clone() ProviderConfig
}