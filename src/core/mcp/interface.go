package mcp

import (
	"context"

	"github.com/sashabaranov/go-openai"
)

type ToolInputSchema struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
	Required   []string       `json:"required,omitempty"`
}

// Tool 表示MCP工具
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema ToolInputSchema `json:"inputSchema"`
}

// MCPClient 定义MCP客户端接口
type MCPClient interface {
	// Start 启动MCP客户端
	Start(ctx context.Context) error

	// Stop 停止MCP客户端
	Stop()

	// HasTool 检查是否有指定名称的工具
	HasTool(name string) bool

	// GetAvailableTools 获取所有可用工具
	GetAvailableTools() []openai.Tool

	// CallTool 调用指定的工具
	CallTool(ctx context.Context, name string, args map[string]interface{}) (interface{}, error)

	// IsReady 检查客户端是否已初始化完成并准备就绪
	IsReady() bool

	// ResetConnection 重置连接状态但保留客户端结构
	ResetConnection() error
}

// 确保Client实现了MCPClient接口
var _ MCPClient = (*Client)(nil)
