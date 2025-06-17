package vlllm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/image"
	"xiaozhi-server-go/src/core/providers"
	"xiaozhi-server-go/src/core/utils"

	"github.com/sashabaranov/go-openai"
)

// Config VLLLM配置结构
type Config struct {
	Type        string
	ModelName   string
	BaseURL     string
	APIKey      string
	Temperature float64
	MaxTokens   int
	TopP        float64
	Security    configs.SecurityConfig
	Data        map[string]interface{}
}

// Provider VLLLM提供者，直接处理多模态API
type Provider struct {
	config         *Config
	imageProcessor *image.ImageProcessor
	logger         *utils.Logger

	// 直接的API客户端
	openaiClient *openai.Client // 用于OpenAI类型
	httpClient   *http.Client   // 用于Ollama类型
}

// OllamaRequest Ollama API请求结构
type OllamaRequest struct {
	Model    string                 `json:"model"`
	Messages []OllamaMessage        `json:"messages"`
	Stream   bool                   `json:"stream"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

// OllamaMessage Ollama消息结构
type OllamaMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"` // base64编码的图片
}

// OllamaResponse Ollama API响应结构
type OllamaResponse struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	Message   struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

// NewProvider 创建新的VLLLM提供者
func NewProvider(config *Config, logger *utils.Logger) (*Provider, error) {
	// 构建VLLLM配置
	vlllmConfig := &configs.VLLMConfig{
		Type:        config.Type,
		ModelName:   config.ModelName,
		BaseURL:     config.BaseURL,
		APIKey:      config.APIKey,
		Temperature: config.Temperature,
		MaxTokens:   config.MaxTokens,
		TopP:        config.TopP,
		Security:    config.Security,
	}

	// 创建图片处理器
	imageProcessor, err := image.NewImageProcessor(vlllmConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("创建图片处理器失败: %v", err)
	}

	provider := &Provider{
		config:         config,
		imageProcessor: imageProcessor,
		logger:         logger,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
	}

	return provider, nil
}

// Initialize 初始化Provider
func (p *Provider) Initialize() error {
	// 根据类型初始化对应的客户端
	switch strings.ToLower(p.config.Type) {
	case "openai":
		if p.config.APIKey == "" {
			return fmt.Errorf("OpenAI API key is required")
		}

		clientConfig := openai.DefaultConfig(p.config.APIKey)
		if p.config.BaseURL != "" {
			clientConfig.BaseURL = p.config.BaseURL
		}
		p.openaiClient = openai.NewClientWithConfig(clientConfig)

	case "ollama":
		// Ollama不需要API key，只需要确保有BaseURL
		if p.config.BaseURL == "" {
			p.config.BaseURL = "http://localhost:11434" // 默认Ollama地址
		}
		p.logger.Debug("Ollama VLLLM初始化成功 %v", map[string]interface{}{
			"base_url": p.config.BaseURL,
			"model":    p.config.ModelName,
		})

	default:
		return fmt.Errorf("不支持的VLLLM类型: %s", p.config.Type)
	}

	p.logger.Debug("VLLLM Provider初始化成功 %v", map[string]interface{}{
		"type":       p.config.Type,
		"model_name": p.config.ModelName,
	})

	return nil
}

// Cleanup 清理资源
func (p *Provider) Cleanup() error {
	// 清理图片处理器
	if err := p.imageProcessor.Cleanup(); err != nil {
		p.logger.Warn("清理图片处理器失败", err)
	}

	p.logger.Info("VLLLM Provider清理完成")
	return nil
}

// ResponseWithImage 处理包含图片的请求 - 核心方法
func (p *Provider) ResponseWithImage(ctx context.Context, sessionID string, messages []providers.Message, imageData image.ImageData, text string) (<-chan string, error) {
	// 处理图片
	base64Image, err := p.imageProcessor.ProcessImage(ctx, imageData)
	if err != nil {
		return nil, fmt.Errorf("图片处理失败: %v", err)
	}

	p.logger.Debug("开始调用多模态API %v", map[string]interface{}{
		"type":       p.config.Type,
		"model_name": p.config.ModelName,
		"text":       text,
		"image_size": len(base64Image),
	})

	// 根据类型调用对应的多模态API
	switch strings.ToLower(p.config.Type) {
	case "openai":
		return p.responseWithOpenAIVision(ctx, messages, base64Image, text, imageData.Format)
	case "ollama":
		return p.responseWithOllamaVision(ctx, messages, base64Image, text, imageData.Format)
	default:
		return nil, fmt.Errorf("不支持的VLLLM类型: %s", p.config.Type)
	}
}

// responseWithOpenAIVision 使用OpenAI Vision API
func (p *Provider) responseWithOpenAIVision(ctx context.Context, messages []providers.Message, base64Image string, text string, format string) (<-chan string, error) {
	responseChan := make(chan string, 10)

	go func() {
		defer close(responseChan)

		// 构建OpenAI多模态消息
		chatMessages := make([]openai.ChatCompletionMessage, 0, len(messages)+1)

		// 添加历史消息
		for _, msg := range messages {
			chatMessages = append(chatMessages, openai.ChatCompletionMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}

		// 构建包含图片的多模态消息
		visionMessage := openai.ChatCompletionMessage{
			Role: openai.ChatMessageRoleUser,
			MultiContent: []openai.ChatMessagePart{
				{
					Type: openai.ChatMessagePartTypeText,
					Text: text,
				},
				{
					Type: openai.ChatMessagePartTypeImageURL,
					ImageURL: &openai.ChatMessageImageURL{
						URL: fmt.Sprintf("data:image/%s;base64,%s", format, base64Image),
					},
				},
			},
		}
		// 打印visionMessage的内容
		p.logger.Debug("构建的OpenAI Vision消息: %v", visionMessage)
		chatMessages = append(chatMessages, visionMessage)

		// 调用OpenAI Vision API
		stream, err := p.openaiClient.CreateChatCompletionStream(
			ctx,
			openai.ChatCompletionRequest{
				Model:       p.config.ModelName,
				Messages:    chatMessages,
				Stream:      true,
				Temperature: float32(p.config.Temperature),
				TopP:        float32(p.config.TopP),
			},
		)
		if err != nil {
			responseChan <- fmt.Sprintf("【VLLLM服务响应异常: %v】", err)
			p.logger.Error("OpenAI Vision API调用失败 %v", err)
			p.logger.Info("OpenAI Vision API调用失败，%s, maxTokens:%dm, Temperature:%f, top:%f", p.config.ModelName, p.config.MaxTokens, float32(p.config.Temperature), float32(p.config.TopP))

			return
		}
		defer stream.Close()

		p.logger.Info("OpenAI Vision API调用成功，开始接收流式回复")

		isActive := true
		for {
			response, err := stream.Recv()
			if err != nil {
				break
			}

			if len(response.Choices) > 0 {
				content := response.Choices[0].Delta.Content
				if content != "" {
					// 处理思考标签
					if content, isActive = p.handleThinkTags(content, isActive); content != "" {
						responseChan <- content
					}
				}
			}
		}

		p.logger.Info("OpenAI Vision API流式回复完成")
	}()

	return responseChan, nil
}

// responseWithOllamaVision 使用Ollama Vision API
func (p *Provider) responseWithOllamaVision(ctx context.Context, messages []providers.Message, base64Image string, text string, format string) (<-chan string, error) {
	responseChan := make(chan string, 10)

	go func() {
		defer close(responseChan)

		// 构建Ollama请求
		ollamaMessages := make([]OllamaMessage, 0, len(messages)+1)

		// 添加历史消息
		for _, msg := range messages {
			ollamaMessages = append(ollamaMessages, OllamaMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}

		// 添加包含图片的用户消息
		visionMessage := OllamaMessage{
			Role:    "user",
			Content: text,
			Images:  []string{base64Image}, // Ollama需要纯base64，不需要data URL前缀
		}
		ollamaMessages = append(ollamaMessages, visionMessage)

		// 构建请求
		request := OllamaRequest{
			Model:    p.config.ModelName,
			Messages: ollamaMessages,
			Stream:   true,
			Options: map[string]interface{}{
				"temperature": p.config.Temperature,
				"top_p":       p.config.TopP,
			},
		}

		// 序列化请求
		requestBody, err := json.Marshal(request)
		if err != nil {
			responseChan <- fmt.Sprintf("【请求序列化失败: %v】", err)
			p.logger.Error("Ollama请求序列化失败", err)
			return
		}

		// 发送请求到Ollama
		url := fmt.Sprintf("%s/api/chat", strings.TrimSuffix(p.config.BaseURL, "/"))
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestBody))
		if err != nil {
			responseChan <- fmt.Sprintf("【创建请求失败: %v】", err)
			p.logger.Error("创建Ollama请求失败", err)
			return
		}

		req.Header.Set("Content-Type", "application/json")

		p.logger.Info("向Ollama发送多模态请求", map[string]interface{}{
			"url":   url,
			"model": p.config.ModelName,
			"text":  text,
		})

		resp, err := p.httpClient.Do(req)
		if err != nil {
			responseChan <- fmt.Sprintf("【Ollama API调用失败: %v】", err)
			p.logger.Error("Ollama API调用失败", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			responseChan <- fmt.Sprintf("【Ollama API返回错误: %d】", resp.StatusCode)
			p.logger.Error("Ollama API返回错误", map[string]interface{}{
				"status_code": resp.StatusCode,
				"status":      resp.Status,
			})
			return
		}

		p.logger.Info("Ollama Vision API调用成功，开始接收流式回复")

		// 处理流式响应
		decoder := json.NewDecoder(resp.Body)
		isActive := true

		for {
			var response OllamaResponse
			if err := decoder.Decode(&response); err != nil {
				if err.Error() != "EOF" {
					p.logger.Error("解析Ollama响应失败", err)
				}
				break
			}

			content := response.Message.Content
			if content != "" {
				// 处理思考标签
				if content, isActive = p.handleThinkTags(content, isActive); content != "" {
					responseChan <- content
				}
			}

			if response.Done {
				break
			}
		}

		p.logger.Info("Ollama Vision API流式回复完成")
	}()

	return responseChan, nil
}

// Response 普通文本响应（降级处理）
func (p *Provider) Response(ctx context.Context, sessionID string, messages []providers.Message) (<-chan string, error) {
	// 如果没有图片，就作为普通文本处理
	responseChan := make(chan string, 1)
	go func() {
		defer close(responseChan)
		responseChan <- "VLLLM Provider只支持图片处理，普通文本请使用LLM Provider"
	}()
	return responseChan, nil
}

// handleThinkTags 处理思考标签
func (p *Provider) handleThinkTags(content string, isActive bool) (string, bool) {
	if content == "" {
		return "", isActive
	}

	if content == "<think>" {
		return "", false
	}
	if content == "</think>" {
		return "", true
	}

	if !isActive {
		return "", isActive
	}

	return content, isActive
}

// detectMultimodalMessage 检测是否为多模态消息（向后兼容）
func (p *Provider) detectMultimodalMessage(content string) (text string, imageURL string, detected bool) {
	// 正则匹配之前的多模态消息格式
	multimodalPattern := regexp.MustCompile(`\[MULTIMODAL_MESSAGE\](.*?)\[/MULTIMODAL_MESSAGE\]`)
	matches := multimodalPattern.FindStringSubmatch(content)

	if len(matches) > 0 {
		// 这是旧格式的多模态消息，需要解析
		// 这里可以添加解析逻辑，但新版本应该直接使用 ResponseWithImage
		return "", "", true
	}

	return content, "", false
}

// GetImageMetrics 获取图片处理统计信息
func (p *Provider) GetImageMetrics() image.ImageMetrics {
	return p.imageProcessor.GetMetrics()
}

// GetConfig 获取配置信息
func (p *Provider) GetConfig() *Config {
	return p.config
}
