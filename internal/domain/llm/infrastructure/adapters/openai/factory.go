package openai

import (
	"fmt"
	"time"

	contractProviders "xiaozhi-server-go/internal/contracts/providers"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/src/core/utils"
)

// OpenAILLMFactory OpenAI LLM提供者工厂
type OpenAILLMFactory struct {
	providerName string
}

// NewOpenAILLMFactory 创建OpenAI LLM工厂
func NewOpenAILLMFactory() contractProviders.LLMProviderFactory {
	return &OpenAILLMFactory{
		providerName: "openai",
	}
}

// GetProviderName 获取提供者名称
func (f *OpenAILLMFactory) GetProviderName() string {
	return f.providerName
}

// ValidateConfig 验证配置
func (f *OpenAILLMFactory) ValidateConfig(config interface{}) error {
	cfg, ok := config.(Config)
	if !ok {
		return fmt.Errorf("invalid config type, expected openai.Config")
	}

	if cfg.APIKey == "" {
		return fmt.Errorf("api_key is required")
	}

	if cfg.Model == "" {
		return fmt.Errorf("model is required")
	}

	// 验证温度参数范围
	if cfg.Temperature < 0 || cfg.Temperature > 2 {
		return fmt.Errorf("temperature must be between 0 and 2")
	}

	// 验证最大token数
	if cfg.MaxTokens < 1 || cfg.MaxTokens > 8192 {
		return fmt.Errorf("max_tokens must be between 1 and 8192")
	}

	return nil
}

// CreateProvider 创建LLM提供者实例
func (f *OpenAILLMFactory) CreateProvider(config interface{}, options map[string]interface{}) (contractProviders.LLMProvider, error) {
	// 解析配置
	var cfg Config

	// 尝试从platform.Config转换为OpenAI配置
	if platformConfig, ok := config.(*config.Config); ok {
		cfg = f.extractFromPlatformConfig(platformConfig)
	} else if openaiConfig, ok := config.(Config); ok {
		cfg = openaiConfig
	} else {
		return nil, fmt.Errorf("unsupported config type for openai LLM provider")
	}

	// 验证配置
	if err := f.ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	// 获取日志记录器
	var logger *utils.Logger
	if loggerVal, ok := options["logger"]; ok {
		if logger, ok = loggerVal.(*utils.Logger); !ok {
			return nil, fmt.Errorf("logger option must be *utils.Logger")
		}
	} else {
		logger = utils.DefaultLogger
	}

	// 创建提供者实例
	provider := NewOpenAILLMProvider(cfg, logger)

	// 初始化提供者
	if err := provider.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize provider: %w", err)
	}

	return provider, nil
}

// extractFromPlatformConfig 从平台配置提取OpenAI配置
func (f *OpenAILLMFactory) extractFromPlatformConfig(platformConfig *config.Config) Config {
	return Config{
		APIKey:      platformConfig.LLM.OpenAI.APIKey,
		BaseURL:     platformConfig.LLM.OpenAI.BaseURL,
		Model:       platformConfig.LLM.OpenAI.Model,
		MaxTokens:   platformConfig.LLM.OpenAI.MaxTokens,
		Temperature: platformConfig.LLM.OpenAI.Temperature,
		Timeout:     time.Duration(platformConfig.LLM.OpenAI.Timeout) * time.Second,
	}
}