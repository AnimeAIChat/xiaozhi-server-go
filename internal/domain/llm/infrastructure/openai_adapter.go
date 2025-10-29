package infrastructure

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/sashabaranov/go-openai"
	"xiaozhi-server-go/internal/domain/llm/aggregate"
	"xiaozhi-server-go/internal/domain/llm/repository"
	"xiaozhi-server-go/internal/platform/errors"
)

type openaiAdapter struct {
	client *openai.Client
}

func NewOpenAIAdapter(apiKey, baseURL string) repository.LLMRepository {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}

	return &openaiAdapter{
		client: openai.NewClientWithConfig(config),
	}
}

func (o *openaiAdapter) Generate(ctx context.Context, req repository.GenerateRequest) (*repository.GenerateResult, error) {
	messages := o.convertMessages(req.Messages)
	tools := o.convertTools(req.Tools)

	chatReq := openai.ChatCompletionRequest{
		Model:    req.Config.Model,
		Messages: messages,
		Tools:    tools,
		Temperature: float32(req.Config.Temperature),
		MaxTokens:   req.Config.MaxTokens,
		TopP:        float32(req.Config.TopP),
	}

	resp, err := o.client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, errors.Wrap(errors.KindDomain, "openai.generate", "API call failed", err)
	}

	if len(resp.Choices) == 0 {
		return nil, errors.New(errors.KindDomain, "openai.generate", "no response choices")
	}

	choice := resp.Choices[0]
	result := &repository.GenerateResult{
		Content:      choice.Message.Content,
		FinishReason: string(choice.FinishReason),
		Usage: &aggregate.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}

	// 处理工具调用
	if choice.Message.ToolCalls != nil {
		result.ToolCalls = make([]repository.ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			result.ToolCalls[i] = repository.ToolCall{
				ID:   tc.ID,
				Type: string(tc.Type),
				Function: repository.ToolCallFunction{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
				Index: *tc.Index,
			}
		}
	}

	return result, nil
}

func (o *openaiAdapter) Stream(ctx context.Context, req repository.GenerateRequest) (<-chan repository.ResponseChunk, error) {
	messages := o.convertMessages(req.Messages)
	tools := o.convertTools(req.Tools)

	chatReq := openai.ChatCompletionRequest{
		Model:    req.Config.Model,
		Messages: messages,
		Tools:    tools,
		Temperature: float32(req.Config.Temperature),
		MaxTokens:   req.Config.MaxTokens,
		TopP:        float32(req.Config.TopP),
		Stream:      true,
	}

	stream, err := o.client.CreateChatCompletionStream(ctx, chatReq)
	if err != nil {
		return nil, errors.Wrap(errors.KindDomain, "openai.stream", "stream creation failed", err)
	}

	outChan := make(chan repository.ResponseChunk, 10)

	go func() {
		defer close(outChan)
		defer stream.Close()

		for {
			response, err := stream.Recv()
			if err != nil {
				if strings.Contains(err.Error(), "stream closed") {
					return
				}
				// 发送错误到通道
				outChan <- repository.ResponseChunk{
					Content: "Error: " + err.Error(),
					Done:    true,
				}
				return
			}

			if len(response.Choices) == 0 {
				continue
			}

			choice := response.Choices[0]
			chunk := repository.ResponseChunk{
				Content: choice.Delta.Content,
				Done:    choice.FinishReason != "",
			}

			// 处理工具调用
			if choice.Delta.ToolCalls != nil {
				chunk.ToolCalls = make([]repository.ToolCall, len(choice.Delta.ToolCalls))
				for i, tc := range choice.Delta.ToolCalls {
					chunk.ToolCalls[i] = repository.ToolCall{
						ID:   tc.ID,
						Type: string(tc.Type),
						Function: repository.ToolCallFunction{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
						Index: *tc.Index,
					}
				}
			}

			// 处理使用情况
			if response.Usage != nil {
				chunk.Usage = &aggregate.Usage{
					PromptTokens:     response.Usage.PromptTokens,
					CompletionTokens: response.Usage.CompletionTokens,
					TotalTokens:      response.Usage.TotalTokens,
				}
			}

			outChan <- chunk

			if chunk.Done {
				break
			}
		}
	}()

	return outChan, nil
}

func (o *openaiAdapter) ValidateConnection(ctx context.Context, config aggregate.Config) error {
	// 发送一个简单的请求来验证连接
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "Hello"},
	}

	req := openai.ChatCompletionRequest{
		Model:    config.Model,
		Messages: messages,
		MaxTokens: 10,
	}

	_, err := o.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return errors.Wrap(errors.KindDomain, "openai.validate", "connection test failed", err)
	}

	return nil
}

func (o *openaiAdapter) GetProviderInfo(provider string) (*repository.ProviderInfo, error) {
	return &repository.ProviderInfo{
		Name: "OpenAI",
		SupportedModels: []string{
			"gpt-4",
			"gpt-4-turbo",
			"gpt-3.5-turbo",
		},
		MaxTokens: 128000,
		Features: []string{
			"chat",
			"tools",
			"streaming",
			"vision",
		},
	}, nil
}

func (o *openaiAdapter) convertMessages(msgs []repository.Message) []openai.ChatCompletionMessage {
	messages := make([]openai.ChatCompletionMessage, len(msgs))
	for i, msg := range msgs {
		messages[i] = openai.ChatCompletionMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			Name:       msg.Name,
			ToolCalls:  o.convertToolCalls(msg.ToolCalls),
			ToolCallID: msg.ToolCallID,
		}
	}
	return messages
}

func (o *openaiAdapter) convertToolCalls(calls []repository.ToolCall) []openai.ToolCall {
	if len(calls) == 0 {
		return nil
	}

	toolCalls := make([]openai.ToolCall, len(calls))
	for i, call := range calls {
		toolCalls[i] = openai.ToolCall{
			ID:   call.ID,
			Type: openai.ToolType(call.Type),
			Function: openai.FunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		}
	}
	return toolCalls
}

func (o *openaiAdapter) convertTools(tools []repository.Tool) []openai.Tool {
	if len(tools) == 0 {
		return nil
	}

	openaiTools := make([]openai.Tool, len(tools))
	for i, tool := range tools {
		params, _ := json.Marshal(tool.Function.Parameters)

		openaiTools[i] = openai.Tool{
			Type: openai.ToolType(tool.Type),
			Function: &openai.FunctionDefinition{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  params,
			},
		}
	}
	return openaiTools
}