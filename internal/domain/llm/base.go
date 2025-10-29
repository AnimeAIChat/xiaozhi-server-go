package llm

import (
	"context"
	"fmt"
	"sync"

	"xiaozhi-server-go/internal/domain/llm/inter"
)

// Manager LLM管理器 - 基于 Eino 框架
type Manager struct {
	mu     sync.RWMutex
	llm    interface{} // Eino LLM component
	config inter.LLMConfig
}

// NewManager 创建LLM管理器
func NewManager(config inter.LLMConfig) *Manager {
	return &Manager{
		config: config,
	}
}

// SetLLM 设置 Eino LLM
func (m *Manager) SetLLM(llm interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.llm = llm
}

// GetLLM 获取 Eino LLM
func (m *Manager) GetLLM() interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.llm
}

// Response 生成回复
func (m *Manager) Response(ctx context.Context, sessionID string, messages []inter.Message, tools []inter.Tool) (<-chan inter.ResponseChunk, error) {
	// TODO: 实现 Eino LLM 调用
	return nil, fmt.Errorf("eino LLM integration not implemented yet")
}

// ResponseWithFunctions 生成带函数调用的回复 (兼容旧接口)
func (m *Manager) ResponseWithFunctions(ctx context.Context, sessionID string, messages []inter.Message, tools []inter.Tool) (<-chan inter.ResponseChunk, error) {
	return m.Response(ctx, sessionID, messages, tools)
}

// Close 关闭LLM资源
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.llm != nil {
		// Eino LLM 的关闭逻辑，如果有的话
		m.llm = nil
	}
	return nil
}

// GetConfig 获取配置
func (m *Manager) GetConfig() inter.LLMConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// UpdateConfig 更新配置
func (m *Manager) UpdateConfig(config inter.LLMConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
}

// ValidateConfig 验证配置
func ValidateConfig(config inter.LLMConfig) error {
	if config.Provider == "" {
		return fmt.Errorf("provider cannot be empty")
	}
	if config.Model == "" {
		return fmt.Errorf("model cannot be empty")
	}
	if config.MaxTokens <= 0 {
		return fmt.Errorf("invalid max tokens: %d", config.MaxTokens)
	}
	if config.Temperature < 0 || config.Temperature > 2 {
		return fmt.Errorf("invalid temperature: %f", config.Temperature)
	}
	if config.Timeout <= 0 {
		return fmt.Errorf("invalid timeout: %d", config.Timeout)
	}
	return nil
}

// DefaultConfig 获取默认配置
func DefaultConfig() inter.LLMConfig {
	return inter.LLMConfig{
		Provider:    "doubao",
		Model:       "doubao-lite-32k",
		Temperature: 0.7,
		MaxTokens:   4096,
		Timeout:     60,
	}
}