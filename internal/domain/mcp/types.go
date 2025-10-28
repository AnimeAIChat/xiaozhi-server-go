package mcp

import (
	"context"

	"github.com/sashabaranov/go-openai"
)

// ToolInputSchema describes the JSON schema for tool arguments.
type ToolInputSchema struct {
	Type       string                   `json:"type"`
	Properties map[string]any           `json:"properties,omitempty"`
	Required   []string                 `json:"required,omitempty"`
	Examples   []map[string]interface{} `json:"examples,omitempty"`
}

// Tool holds the metadata necessary to expose a tool to clients.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema ToolInputSchema `json:"input_schema"`
}

// Client encapsulates the lifecycle of a MCP client implementation.
type Client interface {
	Start(ctx context.Context) error
	Stop()
	HasTool(name string) bool
	GetAvailableTools() []openai.Tool
	CallTool(ctx context.Context, name string, args map[string]any) (any, error)
	IsReady() bool
	ResetConnection() error
}

// ToolExecutor represents an execution primitive that can be plugged into the manager.
type ToolExecutor interface {
	Execute(ctx context.Context, name string, args map[string]any) (any, error)
}

// Logger captures the logging interface consumed by the domain.
type Logger interface {
	Debug(format string, args ...any)
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
}

func (t Tool) toOpenAITool() openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.InputSchema.toParameters(),
		},
	}
}

func (s ToolInputSchema) toParameters() map[string]any {
	params := map[string]any{
		"type": s.Type,
	}
	if len(s.Properties) > 0 {
		params["properties"] = s.Properties
	}
	if len(s.Required) > 0 {
		params["required"] = s.Required
	}
	if len(s.Examples) > 0 {
		params["examples"] = s.Examples
	}
	return params
}
