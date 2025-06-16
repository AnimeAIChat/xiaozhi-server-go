package openai

import (
	"xiaozhi-server-go/src/core/providers/vlllm"
	"xiaozhi-server-go/src/core/utils"
)

// OpenAIVLLMProvider OpenAI类型的VLLLM提供者
type OpenAIVLLMProvider struct {
	*vlllm.Provider
}

// NewProvider 创建OpenAI VLLLM提供者实例
func NewProvider(config *vlllm.Config, logger *utils.Logger) (*vlllm.Provider, error) {
	// 直接使用基础VLLLM Provider，因为它已经复用了LLM架构
	// OpenAI类型的VLLLM只需要确保使用正确的模型名称（如glm-4v-flash）
	provider, err := vlllm.NewProvider(config, logger)
	if err != nil {
		return nil, err
	}

	logger.Debug("OpenAI VLLLM Provider创建成功 %v", map[string]interface{}{
		"model_name": config.ModelName,
		"base_url":   config.BaseURL,
	})

	return provider, nil
}

// init 注册OpenAI VLLLM提供者
func init() {
	vlllm.Register("openai", NewProvider)
}
