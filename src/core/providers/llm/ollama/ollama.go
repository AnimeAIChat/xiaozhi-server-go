package ollama

import (
	"context"
	"fmt"
	"strings"
	"xiaozhi-server-go/src/core/providers/llm"
	"xiaozhi-server-go/src/core/types"

	"github.com/sashabaranov/go-openai"
)

// Provider Ollama LLM提供者
type Provider struct {
	*llm.BaseProvider
	client    *openai.Client
	modelName string
	isQwen3   bool
}

// 注册提供者
func init() {
	llm.Register("ollama", NewProvider)
}

// NewProvider 创建Ollama提供者
func NewProvider(config *llm.Config) (llm.Provider, error) {
	base := llm.NewBaseProvider(config)
	provider := &Provider{
		BaseProvider: base,
		modelName:    config.ModelName,
	}

	// 检查是否是qwen3模型
	provider.isQwen3 = config.ModelName != "" && strings.HasPrefix(strings.ToLower(config.ModelName), "qwen3")

	return provider, nil
}

// Initialize 初始化提供者
func (p *Provider) Initialize() error {
	config := p.Config()
	baseURL := config.BaseURL
	if baseURL == "" {
		// 尝试从url字段获取
		if url, ok := config.Extra["url"].(string); ok {
			baseURL = url
		}
	}
	if baseURL == "" {
		return fmt.Errorf("缺少Ollama基础URL配置")
	}

	// 确保URL以/v1结尾
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL = baseURL + "/v1"
	}

	// Ollama不需要真正的API key，但openai客户端需要一个值
	clientConfig := openai.DefaultConfig("ollama")
	clientConfig.BaseURL = baseURL

	p.client = openai.NewClientWithConfig(clientConfig)
	return nil
}

// Cleanup 清理资源
func (p *Provider) Cleanup() error {
	return nil
}

// Response types.LLMProvider接口实现
func (p *Provider) Response(ctx context.Context, sessionID string, messages []types.Message) (<-chan string, error) {
	responseChan := make(chan string, 10)

	go func() {
		defer close(responseChan)

		// 如果是qwen3模型，在用户最后一条消息中添加/no_think指令
		if p.isQwen3 {
			messages = p.addNoThinkDirective(messages)
		}

		// 转换消息格式
		chatMessages := make([]openai.ChatCompletionMessage, len(messages))
		for i, msg := range messages {
			chatMessages[i] = openai.ChatCompletionMessage{
				Role:    msg.Role,
				Content: msg.Content,
			}
		}

		stream, err := p.client.CreateChatCompletionStream(
			ctx,
			openai.ChatCompletionRequest{
				Model:    p.modelName,
				Messages: chatMessages,
				Stream:   true,
			},
		)
		if err != nil {
			responseChan <- fmt.Sprintf("【Ollama服务响应异常: %v】", err)
			return
		}
		defer stream.Close()

		isActive := true
		buffer := ""

		for {
			response, err := stream.Recv()
			if err != nil {
				break
			}

			if len(response.Choices) > 0 {
				content := response.Choices[0].Delta.Content
				if content != "" {
					// 将内容添加到缓冲区
					buffer += content

					// 处理缓冲区中的标签
					buffer, isActive = p.handleThinkTagsWithBuffer(buffer, isActive)

					// 如果当前处于活动状态且缓冲区有内容，则输出
					if isActive && buffer != "" {
						responseChan <- buffer
						buffer = ""
					}
				}
			}
		}
	}()

	return responseChan, nil
}

// ResponseWithFunctions types.LLMProvider接口实现
func (p *Provider) ResponseWithFunctions(ctx context.Context, sessionID string, messages []types.Message, tools []openai.Tool) (<-chan types.Response, error) {
	responseChan := make(chan types.Response, 10)

	go func() {
		defer close(responseChan)

		// 如果是qwen3模型，在用户最后一条消息中添加/no_think指令
		if p.isQwen3 {
			messages = p.addNoThinkDirective(messages)
		}

		// 转换消息格式
		chatMessages := make([]openai.ChatCompletionMessage, len(messages))
		for i, msg := range messages {
			chatMessage := openai.ChatCompletionMessage{
				Role:    msg.Role,
				Content: msg.Content,
			}

			// 处理tool_call_id字段（tool消息必需）
			if msg.ToolCallID != "" {
				chatMessage.ToolCallID = msg.ToolCallID
			}

			// 处理tool_calls字段（assistant消息中的工具调用）
			if len(msg.ToolCalls) > 0 {
				openaiToolCalls := make([]openai.ToolCall, len(msg.ToolCalls))
				for j, tc := range msg.ToolCalls {
					openaiToolCalls[j] = openai.ToolCall{
						ID:   tc.ID,
						Type: openai.ToolType(tc.Type),
						Function: openai.FunctionCall{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					}
				}
				chatMessage.ToolCalls = openaiToolCalls
			}

			chatMessages[i] = chatMessage
		}

		stream, err := p.client.CreateChatCompletionStream(
			ctx,
			openai.ChatCompletionRequest{
				Model:    p.modelName,
				Messages: chatMessages,
				Tools:    tools,
				Stream:   true,
			},
		)
		if err != nil {
			responseChan <- types.Response{
				Content: fmt.Sprintf("【Ollama服务响应异常: %v】", err),
				Error:   err.Error(),
			}
			return
		}
		defer stream.Close()

		isActive := true
		buffer := ""

		for {
			response, err := stream.Recv()
			if err != nil {
				break
			}

			if len(response.Choices) > 0 {
				delta := response.Choices[0].Delta

				// 处理工具调用
				if len(delta.ToolCalls) > 0 {
					toolCalls := make([]types.ToolCall, len(delta.ToolCalls))
					for i, tc := range delta.ToolCalls {
						toolCalls[i] = types.ToolCall{
							ID:   tc.ID,
							Type: string(tc.Type),
							Function: types.FunctionCall{
								Name:      tc.Function.Name,
								Arguments: tc.Function.Arguments,
							},
						}
					}
					responseChan <- types.Response{
						ToolCalls: toolCalls,
					}
					continue
				}

				// 处理文本内容
				if delta.Content != "" {
					// 将内容添加到缓冲区
					buffer += delta.Content

					// 处理缓冲区中的标签
					buffer, isActive = p.handleThinkTagsWithBuffer(buffer, isActive)

					// 如果当前处于活动状态且缓冲区有内容，则输出
					if isActive && buffer != "" {
						responseChan <- types.Response{
							Content: buffer,
						}
						buffer = ""
					}
				}
			}
		}
	}()

	return responseChan, nil
}

// addNoThinkDirective 为qwen3模型在用户最后一条消息中添加/no_think指令
func (p *Provider) addNoThinkDirective(messages []types.Message) []types.Message {
	// 复制消息列表
	messagesCopy := make([]types.Message, len(messages))
	copy(messagesCopy, messages)

	// 找到最后一条用户消息
	for i := len(messagesCopy) - 1; i >= 0; i-- {
		if messagesCopy[i].Role == "user" {
			// 在用户消息前添加/no_think指令
			messagesCopy[i].Content = "/no_think " + messagesCopy[i].Content
			break
		}
	}

	return messagesCopy
}

// handleThinkTagsWithBuffer 处理思考标签并返回处理后的缓冲区和活动状态
func (p *Provider) handleThinkTagsWithBuffer(buffer string, isActive bool) (string, bool) {
	if buffer == "" {
		return buffer, isActive
	}

	// 处理完整的<think></think>标签
	for strings.Contains(buffer, "<think>") && strings.Contains(buffer, "</think>") {
		parts := strings.SplitN(buffer, "<think>", 2)
		pre := parts[0]
		parts = strings.SplitN(parts[1], "</think>", 2)
		post := parts[1]
		buffer = pre + post
	}

	// 处理只有开始标签的情况
	if strings.Contains(buffer, "<think>") {
		parts := strings.SplitN(buffer, "<think>", 2)
		buffer = parts[0]
		isActive = false
	}

	// 处理只有结束标签的情况
	if strings.Contains(buffer, "</think>") {
		parts := strings.SplitN(buffer, "</think>", 2)
		buffer = parts[1]
		isActive = true
	}

	return buffer, isActive
}
