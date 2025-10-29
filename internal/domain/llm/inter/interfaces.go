package inter

import "context"

// LLMProvider LLM提供者接口
type LLMProvider interface {
	// Response 生成回复
	Response(ctx context.Context, sessionID string, messages []Message, tools []Tool) (<-chan ResponseChunk, error)

	// ResponseWithFunctions 生成带函数调用的回复
	ResponseWithFunctions(ctx context.Context, sessionID string, messages []Message, tools []Tool) (<-chan ResponseChunk, error)

	// GetConfig 获取LLM配置
	GetConfig() LLMConfig

	// Close 关闭LLM资源
	Close() error
}

// Message LLM消息
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	Name       string     `json:"name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// Tool LLM工具
type Tool struct {
	Type     string     `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction 工具函数定义
type ToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// ResponseChunk 回复块
type ResponseChunk struct {
	Content      string                 `json:"content"`
	ToolCalls    []ToolCall             `json:"tool_calls,omitempty"`
	IsDone       bool                   `json:"is_done"`
	Error        error                  `json:"error,omitempty"`
	Usage        *Usage                 `json:"usage,omitempty"`
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction 工具调用函数
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Usage Token使用统计
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// LLMConfig LLM配置
type LLMConfig struct {
	Provider    string  `json:"provider"`     // 提供者类型 (openai, doubao, etc.)
	Model       string  `json:"model"`        // 模型名称
	APIKey      string  `json:"api_key"`      // API密钥
	BaseURL     string  `json:"base_url"`     // 基础URL
	Temperature float32 `json:"temperature"`  // 温度参数
	MaxTokens   int     `json:"max_tokens"`   // 最大token数
	Timeout     int     `json:"timeout"`      // 超时时间(秒)
}