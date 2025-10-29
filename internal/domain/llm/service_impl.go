package llm

import (
	"context"
	"xiaozhi-server-go/internal/domain/llm/aggregate"
	"xiaozhi-server-go/internal/domain/llm/repository"
	"xiaozhi-server-go/internal/platform/errors"
)

type serviceImpl struct {
	repo repository.LLMRepository
}

func NewService(repo repository.LLMRepository) Service {
	return &serviceImpl{repo: repo}
}

func (s *serviceImpl) GenerateResponse(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if err := s.ValidateConfig(req.Config); err != nil {
		return nil, errors.Wrap(errors.KindDomain, "generate", "config validation failed", err)
	}

	result, err := s.repo.Generate(ctx, repository.GenerateRequest{
		SessionID: req.SessionID,
		Messages:  convertMessages(req.Messages),
		Tools:     convertTools(req.Tools),
		Config:    req.Config,
	})

	if err != nil {
		return nil, errors.Wrap(errors.KindDomain, "generate", "repository call failed", err)
	}

	return &GenerateResponse{
		Content:   result.Content,
		ToolCalls: convertToolCallsToAggregate(result.ToolCalls),
		Usage:     *result.Usage,
		FinishReason: result.FinishReason,
	}, nil
}

func (s *serviceImpl) StreamResponse(ctx context.Context, req GenerateRequest) (<-chan ResponseChunk, error) {
	if err := s.ValidateConfig(req.Config); err != nil {
		return nil, errors.Wrap(errors.KindDomain, "stream", "config validation failed", err)
	}

	stream, err := s.repo.Stream(ctx, repository.GenerateRequest{
		SessionID: req.SessionID,
		Messages:  convertMessages(req.Messages),
		Tools:     convertTools(req.Tools),
		Config:    req.Config,
	})

	if err != nil {
		return nil, errors.Wrap(errors.KindDomain, "stream", "repository call failed", err)
	}

	outChan := make(chan ResponseChunk, 10)

	go func() {
		defer close(outChan)

		for chunk := range stream {
			outChan <- ResponseChunk{
				Content:   chunk.Content,
				ToolCalls: convertToolCallsToAggregate(chunk.ToolCalls),
				Done:      chunk.Done,
				Usage:     chunk.Usage,
			}
		}
	}()

	return outChan, nil
}

func (s *serviceImpl) ValidateConfig(config aggregate.Config) error {
	if config.Provider == "" {
		return errors.New(errors.KindDomain, "validate", "provider cannot be empty")
	}

	if config.Model == "" {
		return errors.New(errors.KindDomain, "validate", "model cannot be empty")
	}

	if config.MaxTokens <= 0 {
		return errors.New(errors.KindDomain, "validate", "max_tokens must be positive")
	}

	if config.Temperature < 0 || config.Temperature > 2 {
		return errors.New(errors.KindDomain, "validate", "temperature must be between 0 and 2")
	}

	return nil
}

func (s *serviceImpl) GetSupportedProviders() []string {
	return []string{"openai", "doubao", "ollama", "coze"}
}

func convertMessages(domainMsgs []aggregate.Message) []repository.Message {
	msgs := make([]repository.Message, len(domainMsgs))
	for i, msg := range domainMsgs {
		msgs[i] = repository.Message{
			ID:        msg.ID,
			Role:      msg.Role,
			Content:   msg.Content,
			Name:      msg.Name,
			ToolCalls: convertToolCalls(msg.ToolCalls),
			ToolCallID: msg.ToolCallID,
			Timestamp: msg.Timestamp,
		}
	}
	return msgs
}

func convertToolCalls(domainCalls []aggregate.ToolCall) []repository.ToolCall {
	calls := make([]repository.ToolCall, len(domainCalls))
	for i, call := range domainCalls {
		calls[i] = repository.ToolCall{
			ID:   call.ID,
			Type: call.Type,
			Function: repository.ToolCallFunction{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
			Index: call.Index,
		}
	}
	return calls
}

func convertTools(domainTools []aggregate.Tool) []repository.Tool {
	tools := make([]repository.Tool, len(domainTools))
	for i, tool := range domainTools {
		tools[i] = repository.Tool{
			ID:   tool.ID,
			Type: tool.Type,
			Function: repository.ToolFunction{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
			Description: tool.Description,
		}
	}
	return tools
}

func convertToolCallsToAggregate(repoCalls []repository.ToolCall) []aggregate.ToolCall {
	calls := make([]aggregate.ToolCall, len(repoCalls))
	for i, call := range repoCalls {
		calls[i] = aggregate.ToolCall{
			ID:   call.ID,
			Type: call.Type,
			Function: aggregate.ToolCallFunction{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
			Index: call.Index,
		}
	}
	return calls
}