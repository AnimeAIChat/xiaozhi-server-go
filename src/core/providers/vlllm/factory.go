package vlllm

import (
	"fmt"

	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/utils"
)

// Factory VLLLM工厂函数类型
type Factory func(config *Config, logger *utils.Logger) (*Provider, error)

var (
	factories = make(map[string]Factory)
)

// Register 注册VLLLM提供者工厂
func Register(name string, factory Factory) {
	factories[name] = factory
}

// Create 创建VLLLM提供者实例
func Create(name string, vlllmConfig *configs.VLLMConfig, logger *utils.Logger) (*Provider, error) {
	factory, ok := factories[name]
	if !ok {
		return nil, fmt.Errorf("未知的VLLLM提供者: %s", name)
	}

	// 转换配置格式
	config := &Config{
		Type:        vlllmConfig.Type,
		ModelName:   vlllmConfig.ModelName,
		BaseURL:     vlllmConfig.BaseURL,
		APIKey:      vlllmConfig.APIKey,
		Temperature: vlllmConfig.Temperature,
		MaxTokens:   vlllmConfig.MaxTokens,
		TopP:        vlllmConfig.TopP,
		Security:    vlllmConfig.Security,
		Data:        vlllmConfig.Extra,
	}

	// 创建提供者实例
	provider, err := factory(config, logger)
	if err != nil {
		return nil, fmt.Errorf("创建VLLLM提供者失败: %v", err)
	}

	// 初始化提供者
	if err := provider.Initialize(); err != nil {
		return nil, fmt.Errorf("初始化VLLLM提供者失败: %v", err)
	}

	logger.Debug("VLLLM提供者创建成功 %v", map[string]interface{}{
		"name":       name,
		"type":       config.Type,
		"model_name": config.ModelName,
	})

	return provider, nil
}

// GetRegisteredProviders 获取已注册的提供者列表
func GetRegisteredProviders() []string {
	var providers []string
	for name := range factories {
		providers = append(providers, name)
	}
	return providers
}
