package repository

import (
	"context"
	"time"
	"xiaozhi-server-go/internal/domain/llm/aggregate"
)

type LLMRepository interface {
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResult, error)
	Stream(ctx context.Context, req GenerateRequest) (<-chan ResponseChunk, error)
	ValidateConnection(ctx context.Context, config aggregate.Config) error
	GetProviderInfo(provider string) (*ProviderInfo, error)
}

type GenerateRequest struct {
	SessionID string
	Messages  []Message
	Tools     []Tool
	Config    aggregate.Config
}

type GenerateResult struct {
	Content      string
	ToolCalls    []ToolCall
	Usage        *aggregate.Usage
	FinishReason string
}

type ResponseChunk struct {
	Content   string
	ToolCalls []ToolCall
	Done      bool
	Usage     *aggregate.Usage
}

type Message struct {
	ID        string
	Role      string
	Content   string
	Name      string
	ToolCalls []ToolCall
	ToolCallID string
	Timestamp time.Time
}

type ToolCall struct {
	ID       string
	Type     string
	Function ToolCallFunction
	Index    int
}

type ToolCallFunction struct {
	Name      string
	Arguments string
}

type Tool struct {
	ID          string
	Type        string
	Function    ToolFunction
	Description string
}

type ToolFunction struct {
	Name        string
	Description string
	Parameters  interface{}
}

type ProviderInfo struct {
	Name         string
	SupportedModels []string
	MaxTokens    int
	Features     []string
}