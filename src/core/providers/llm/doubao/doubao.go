package doubao

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"xiaozhi-server-go/src/core/providers"
	"xiaozhi-server-go/src/core/providers/llm"
	"xiaozhi-server-go/internal/domain/llm/inter"

	"github.com/sashabaranov/go-openai"
)

// Provider Doubao LLM提供者
type Provider struct {
	*llm.BaseProvider
	client         *http.Client
	apiKey         string
	baseURL        string
	maxTokens      int
	thinkingType   string // thinking类型: "disabled"(关闭思考) 或 "enabled"(启用思考)
}

// doubaoRequest 自定义请求结构体,支持thinking参数
type doubaoRequest struct {
	Model       string                          `json:"model"`
	Messages    []map[string]interface{}        `json:"messages"`
	Stream      bool                            `json:"stream"`
	MaxTokens   int                             `json:"max_tokens,omitempty"`
	Temperature float64                         `json:"temperature,omitempty"`
	TopP        float64                         `json:"top_p,omitempty"`
	Tools       []openai.Tool                   `json:"tools,omitempty"`
	Thinking    map[string]string               `json:"thinking,omitempty"` // 支持thinking参数
}

// doubaoStreamResponse SSE响应结构
type doubaoStreamResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role      string              `json:"role,omitempty"`
			Content   string              `json:"content,omitempty"`
			ToolCalls []openai.ToolCall   `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
}

// 注册提供者
func init() {
	llm.Register("doubao", NewProvider)
}

// NewProvider 创建Doubao提供者
func NewProvider(config *llm.Config) (llm.Provider, error) {
	base := llm.NewBaseProvider(config)
	provider := &Provider{
		BaseProvider: base,
		maxTokens:    config.MaxTokens,
		thinkingType: "disabled", // 默认关闭思考
	}
	if provider.maxTokens <= 0 {
		provider.maxTokens = 500
	}

	// 从Extra字段读取thinking配置
	if config.Extra != nil {
		if thinking, ok := config.Extra["thinking"].(string); ok {
			provider.thinkingType = thinking
		}
	}

	return provider, nil
}

// Initialize 初始化提供者
func (p *Provider) Initialize() error {
	config := p.Config()
	if config.APIKey == "" {
		return fmt.Errorf("missing Doubao API key")
	}

	p.apiKey = config.APIKey
	p.client = &http.Client{}
	
	// Doubao使用火山引擎的API地址
	if config.BaseURL != "" {
		p.baseURL = config.BaseURL
	} else {
		p.baseURL = "https://ark.cn-beijing.volces.com/api/v3"
	}

	return nil
}

// Cleanup 清理资源
func (p *Provider) Cleanup() error {
	return nil
}

// Response types.LLMProvider接口实现
func (p *Provider) Response(ctx context.Context, sessionID string, messages []providers.Message) (<-chan string, error) {
	responseChan := make(chan string, 10)

	go func() {
		defer close(responseChan)

		// 转换消息格式
		reqMessages := make([]map[string]interface{}, len(messages))
		for i, msg := range messages {
			reqMessages[i] = map[string]interface{}{
				"role":    msg.Role,
				"content": msg.Content,
			}
		}

		// 构建自定义请求
		reqBody := doubaoRequest{
			Model:     p.Config().ModelName,
			Messages:  reqMessages,
			Stream:    true,
			MaxTokens: p.maxTokens,
		}

		// 添加thinking参数
		if p.thinkingType != "" {
			reqBody.Thinking = map[string]string{
				"type": p.thinkingType,
			}
		}

		// 序列化请求
		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			responseChan <- fmt.Sprintf("【请求序列化失败: %v】", err)
			return
		}

		// 调试: 打印请求体(可选,用于调试)
		// fmt.Printf("Doubao请求体: %s\n", string(jsonData))

		// 创建HTTP请求
		url := fmt.Sprintf("%s/chat/completions", p.baseURL)
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			responseChan <- fmt.Sprintf("【创建请求失败: %v】", err)
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))

		// 发送请求
		resp, err := p.client.Do(req)
		if err != nil {
			responseChan <- fmt.Sprintf("【Doubao服务响应异常: %v】", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			responseChan <- fmt.Sprintf("【Doubao服务错误 %d: %s】", resp.StatusCode, string(body))
			return
		}

		// 读取SSE流
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					responseChan <- fmt.Sprintf("【读取响应失败: %v】", err)
				}
				break
			}

			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}

			// SSE格式: "data: {...}"
			if bytes.HasPrefix(line, []byte("data: ")) {
				data := bytes.TrimPrefix(line, []byte("data: "))
				
				// 检查是否是结束标记
				if string(data) == "[DONE]" {
					break
				}

				// 解析JSON
				var streamResp doubaoStreamResponse
				if err := json.Unmarshal(data, &streamResp); err != nil {
					continue
				}

				// 提取内容
				if len(streamResp.Choices) > 0 {
					content := streamResp.Choices[0].Delta.Content
					if content != "" {
						responseChan <- content
					}
				}
			}
		}
	}()

	return responseChan, nil
}

// ResponseWithFunctions types.LLMProvider接口实现
func (p *Provider) ResponseWithFunctions(ctx context.Context, sessionID string, messages []providers.Message, tools []providers.Tool) (<-chan providers.Response, error) {
	responseChan := make(chan providers.Response, 10)

	go func() {
		defer close(responseChan)

		// 转换消息格式
		reqMessages := make([]map[string]interface{}, len(messages))
		for i, msg := range messages {
			msgMap := map[string]interface{}{
				"role":    msg.Role,
				"content": msg.Content,
			}

			// 处理tool_call_id字段（tool消息必需）
			if msg.ToolCallID != "" {
				msgMap["tool_call_id"] = msg.ToolCallID
			}

			// 处理tool_calls字段（assistant消息中的工具调用）
			if len(msg.ToolCalls) > 0 {
				toolCalls := make([]map[string]interface{}, len(msg.ToolCalls))
				for j, tc := range msg.ToolCalls {
					toolCalls[j] = map[string]interface{}{
						"id":   tc.ID,
						"type": tc.Type,
						"function": map[string]interface{}{
							"name":      tc.Function.Name,
							"arguments": tc.Function.Arguments,
						},
					}
				}
				msgMap["tool_calls"] = toolCalls
			}

			reqMessages[i] = msgMap
		}

		// 转换工具格式
		openaiTools := make([]openai.Tool, len(tools))
		for i, tool := range tools {
			openaiTools[i] = openai.Tool{
				Type: openai.ToolType(tool.Type),
				Function: &openai.FunctionDefinition{
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					Parameters:  tool.Function.Parameters,
				},
			}
		}

		// 构建自定义请求
		reqBody := doubaoRequest{
			Model:    p.Config().ModelName,
			Messages: reqMessages,
			Tools:    openaiTools,
			Stream:   true,
		}

		// 添加thinking参数
		if p.thinkingType != "" {
			reqBody.Thinking = map[string]string{
				"type": p.thinkingType,
			}
		}

		// 序列化请求
		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			responseChan <- providers.Response{
				Content: fmt.Sprintf("【请求序列化失败: %v】", err),
				Error:   err,
			}
			return
		}

		// 调试: 打印请求体(可选,用于调试)
		// fmt.Printf("Doubao请求体(带工具): %s\n", string(jsonData))

		// 创建HTTP请求
		url := fmt.Sprintf("%s/chat/completions", p.baseURL)
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			responseChan <- providers.Response{
				Content: fmt.Sprintf("【创建请求失败: %v】", err),
				Error:   err,
			}
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))

		// 发送请求
		resp, err := p.client.Do(req)
		if err != nil {
			responseChan <- providers.Response{
				Content: fmt.Sprintf("【Doubao服务响应异常: %v】", err),
				Error:   err,
			}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			responseChan <- providers.Response{
				Content: fmt.Sprintf("【Doubao服务错误 %d: %s】", resp.StatusCode, string(body)),
				Error:   fmt.Errorf("HTTP %d", resp.StatusCode),
			}
			return
		}

		// 读取SSE流
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					responseChan <- providers.Response{
						Content: fmt.Sprintf("【读取响应失败: %v】", err),
						Error:   err,
					}
				}
				break
			}

			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}

			// SSE格式: "data: {...}"
			if bytes.HasPrefix(line, []byte("data: ")) {
				data := bytes.TrimPrefix(line, []byte("data: "))
				
				// 检查是否是结束标记
				if string(data) == "[DONE]" {
					break
				}

				// 解析JSON
				var streamResp doubaoStreamResponse
				if err := json.Unmarshal(data, &streamResp); err != nil {
					continue
				}

				// 提取内容和工具调用
				if len(streamResp.Choices) > 0 {
					delta := streamResp.Choices[0].Delta

					// 处理工具调用
					if len(delta.ToolCalls) > 0 {
						toolCalls := make([]inter.ToolCall, len(delta.ToolCalls))
						for i, tc := range delta.ToolCalls {
							toolCalls[i] = inter.ToolCall{
								ID:   tc.ID,
								Type: string(tc.Type),
								Function: inter.ToolCallFunction{
									Name:      tc.Function.Name,
									Arguments: tc.Function.Arguments,
								},
							}
						}
						responseChan <- providers.Response{
							ToolCalls: toolCalls,
						}
						continue
					}

					// 处理文本内容
					if delta.Content != "" {
						// 暂时输出原始内容，不进行过滤
						responseChan <- providers.Response{
							Content: delta.Content,
						}
					}
				}
			}
		}
	}()

	return responseChan, nil
}
