package llm

import (
	"context"
	"fmt"
	"sync"

	"xiaozhi-server-go/internal/domain/llm/inter"
	"xiaozhi-server-go/src/core/providers/llm"
	"xiaozhi-server-go/src/core/types"

	"github.com/sashabaranov/go-openai"
)

// Manager LLM管理器 - 基于 Eino 框架
type Manager struct {
	mu     sync.RWMutex
	llm    interface{} // Eino LLM component
	config inter.LLMConfig
	provider inter.LLMProvider // 实际的LLM提供商
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
	// 创建LLM配置
	llmConfig := &llm.Config{
		Type:        m.config.Provider,
		ModelName:   m.config.Model,
		BaseURL:     m.config.BaseURL,
		APIKey:      m.config.APIKey,
		Temperature: float64(m.config.Temperature),
		MaxTokens:   m.config.MaxTokens,
	}

	// 创建LLM提供商
	provider, err := llm.Create(m.config.Provider, llmConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM provider: %w", err)
	}

	// 初始化提供商
	if err := provider.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize LLM provider: %w", err)
	}

	// 转换消息格式
	coreMessages := make([]types.Message, len(messages))
	for i, msg := range messages {
		coreMessages[i] = types.Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// 转换工具格式
	coreTools := make([]openai.Tool, len(tools))
	for i, tool := range tools {
		coreTools[i] = openai.Tool{
			Type: openai.ToolType(tool.Type),
			Function: &openai.FunctionDefinition{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		}
	}

	// 调用提供商的ResponseWithFunctions方法
	responseChan, err := provider.ResponseWithFunctions(ctx, sessionID, coreMessages, coreTools)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM response: %w", err)
	}

	// 创建输出通道
	outChan := make(chan inter.ResponseChunk, 10)

	// 启动goroutine转换响应格式
	go func() {
		defer close(outChan)
		defer provider.Cleanup()

		for response := range responseChan {
			chunk := inter.ResponseChunk{
				Content: response.Content,
				IsDone:  response.StopReason != "", // 假设有StopReason表示完成
				// Usage字段在types.Response中不存在，暂时设为nil
				Usage: nil,
			}

			// 转换工具调用
			if len(response.ToolCalls) > 0 {
				chunk.ToolCalls = make([]inter.ToolCall, len(response.ToolCalls))
				for i, tc := range response.ToolCalls {
					chunk.ToolCalls[i] = inter.ToolCall{
						ID:   tc.ID,
						Type: tc.Type,
						Function: inter.ToolCallFunction{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					}
				}
			}

			// 检查是否完成
			if response.StopReason != "" {
				chunk.IsDone = true
			}

			outChan <- chunk

			// 如果是最后一块，退出
			if chunk.IsDone {
				break
			}
		}
	}()

	return outChan, nil
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