package providers

import (
	"context"
)

// Message LLM消息
type Message struct {
	Role       string      `json:"role"`        // 系统角色: system, user, assistant
	Content    string      `json:"content"`     // 消息内容
	Name       string      `json:"name,omitempty"` // 消息发送者名称
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"` // 工具调用列表
	ToolCallID string      `json:"tool_call_id,omitempty"` // 工具调用ID
}

// Tool LLM工具定义
type Tool struct {
	Type     string       `json:"type"`        // 工具类型: function
	Function ToolFunction `json:"function"`    // 工具函数定义
}

// ToolFunction 工具函数定义
type ToolFunction struct {
	Name        string      `json:"name"`         // 函数名称
	Description string      `json:"description"`  // 函数描述
	Parameters  interface{} `json:"parameters"`   // 函数参数模式
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string           `json:"id"`           // 调用ID
	Type     string           `json:"type"`         // 调用类型
	Function ToolCallFunction `json:"function"`     // 函数调用信息
}

// ToolCallFunction 工具调用函数
type ToolCallFunction struct {
	Name      string `json:"name"`       // 函数名称
	Arguments string `json:"arguments"`  // 函数参数JSON字符串
}

// ResponseChunk LLM响应块
type ResponseChunk struct {
	Content      string            `json:"content"`       // 响应内容
	ToolCalls    []ToolCall        `json:"tool_calls,omitempty"` // 工具调用结果
	IsDone       bool              `json:"is_done"`       // 是否完成
	Error        error             `json:"error,omitempty"` // 错误信息
	Usage        *Usage            `json:"usage,omitempty"` // Token使用统计
	Metadata     map[string]interface{} `json:"metadata,omitempty"` // 元数据
}

// Usage Token使用统计
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`     // 提示Token数
	CompletionTokens int `json:"completion_tokens"` // 完成Token数
	TotalTokens      int `json:"total_tokens"`      // 总Token数
}

// LLMProvider 大语言模型提供者统一接口
type LLMProvider interface {
	BaseProvider

	// Response 生成回复（基础模式）
	Response(ctx context.Context, sessionID string, messages []Message) (<-chan ResponseChunk, error)

	// ResponseWithFunctions 生成带函数调用的回复
	ResponseWithFunctions(
		ctx context.Context,
		sessionID string,
		messages []Message,
		tools []Tool,
	) (<-chan ResponseChunk, error)

	// ResponseWithTools 生成带工具的回复（与WithFunctions相同，更明确的命名）
	ResponseWithTools(
		ctx context.Context,
		sessionID string,
		messages []Message,
		tools []Tool,
	) (<-chan ResponseChunk, error)

	// GetCapabilities 获取提供者能力
	GetCapabilities() LLMCapabilities

	// SetIdentityFlag 设置身份标识（用于多租户等场景）
	SetIdentityFlag(idType string, flag string)

	// GetConfig 获取LLM配置
	GetConfig() LLMConfig
}

// LLMCapabilities LLM提供者能力
type LLMCapabilities struct {
	// SupportStreaming 是否支持流式响应
	SupportStreaming bool `json:"support_streaming"`

	// SupportFunctions 是否支持函数调用
	SupportFunctions bool `json:"support_functions"`

	// SupportVision 是否支持视觉输入
	SupportVision bool `json:"support_vision"`

	// MaxTokens 最大Token数限制
	MaxTokens int `json:"max_tokens"`

	// SupportedModels 支持的模型列表
	SupportedModels []string `json:"supported_models"`
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

	// 扩展配置
	Extra map[string]interface{} `json:"extra,omitempty"`
}