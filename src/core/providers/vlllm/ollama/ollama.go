package ollama

import (
	"xiaozhi-server-go/src/core/providers/vlllm"
	"xiaozhi-server-go/src/core/utils"
)

// OllamaVLLMProvider Ollama类型的VLLLM提供者
type OllamaVLLMProvider struct {
	*vlllm.Provider
}

// NewProvider 创建Ollama VLLLM提供者实例
func NewProvider(config *vlllm.Config, logger *utils.Logger) (*vlllm.Provider, error) {
	// 直接使用基础VLLLM Provider，因为它已经复用了LLM架构
	// Ollama类型的VLLLM只需要确保使用正确的模型名称（如qwen2-vl:7b）
	provider, err := vlllm.NewProvider(config, logger)
	if err != nil {
		return nil, err
	}

	logger.Debug("Ollama VLLLM Provider创建成功 %v", map[string]interface{}{
		"model_name": config.ModelName,
		"base_url":   config.BaseURL,
	})

	return provider, nil
}

// init 注册Ollama VLLLM提供者
func init() {
	vlllm.Register("ollama", NewProvider)
}
