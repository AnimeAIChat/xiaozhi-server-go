package llm

import (
	"context"
	"xiaozhi-server-go/internal/domain/llm/aggregate"
)

type Service interface {
	GenerateResponse(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
	StreamResponse(ctx context.Context, req GenerateRequest) (<-chan ResponseChunk, error)
	ValidateConfig(config aggregate.Config) error
	GetSupportedProviders() []string
}

type GenerateRequest struct {
	SessionID string
	Messages  []aggregate.Message
	Tools     []aggregate.Tool
	Config    aggregate.Config
}

type GenerateResponse struct {
	Content   string
	ToolCalls []aggregate.ToolCall
	Usage     aggregate.Usage
	FinishReason string
}

type ResponseChunk struct {
	Content   string
	ToolCalls []aggregate.ToolCall
	Done      bool
	Usage     *aggregate.Usage
}