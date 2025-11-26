package vlllm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	domainimage "xiaozhi-server-go/internal/domain/image"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/internal/core/providers"
	"xiaozhi-server-go/internal/utils"

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
	Security    config.SecurityConfig
	Data        map[string]interface{}
}

// Provider VLLLM提供者，直接处理多模态API
type Provider struct {
	config        *Config
	imagePipeline *domainimage.Pipeline
	security      config.SecurityConfig
	logger        *utils.Logger

	openaiClient *openai.Client
	httpClient   *http.Client
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
	security := config.Security
	imagePipeline, err := domainimage.NewPipeline(domainimage.Options{
		Security: &security,
		Logger:   logger,
	})
	if err != nil {
		return nil, fmt.Errorf("initialise image pipeline: %w", err)
	}

	provider := &Provider{
		config:        config,
		security:      security,
		imagePipeline: imagePipeline,
		logger:        logger,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
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
		p.logger.Debug(
			"Ollama VLLLM初始化成功: base_url=%s model=%s",
			p.config.BaseURL,
			p.config.ModelName,
		)

	default:
		return fmt.Errorf("不支持的VLLLM类型: %s", p.config.Type)
	}

	p.logger.Debug(
		"VLLLM Provider初始化成功: type=%s model_name=%s",
		p.config.Type,
		p.config.ModelName,
	)

	return nil
}

// Cleanup 释放资源
func (p *Provider) Cleanup() error {
	p.logger.Info("VLLLM Provider cleaned up")
	return nil
}

// ResponseWithImage 处理包含图片的请求 - 核心方法
func (p *Provider) ResponseWithImage(ctx context.Context, sessionID string, messages []providers.Message, imageData domainimage.ImageData, text string) (<-chan string, error) {
	// 处理图片
	output, err := p.prepareImagePayload(ctx, imageData)
	if err != nil {
		return nil, fmt.Errorf("image pipeline: %w", err)
	}

	base64Image := output.Base64
	format := output.Validation.Format

	p.logger.Debug(
		"invoke vision API: type=%s model_name=%s text_length=%d image_bytes=%d",
		p.config.Type,
		p.config.ModelName,
		len(text),
		len(output.Bytes),
	)

	switch strings.ToLower(p.config.Type) {
	case "openai":
		return p.responseWithOpenAIVision(ctx, messages, base64Image, text, format)
	case "ollama":
		return p.responseWithOllamaVision(ctx, messages, base64Image, text, format)
	default:
		return nil, fmt.Errorf("unsupported VLLLM provider: %s", p.config.Type)
	}
}
func (p *Provider) prepareImagePayload(ctx context.Context, payload domainimage.ImageData) (*domainimage.Output, error) {
	var (
		reader      io.ReadCloser
		formatHint  = payload.Format
		sourceLabel string
		err         error
	)

	switch {
	case payload.URL != "":
		sourceLabel = payload.URL
		reader, formatHint, err = p.downloadImage(ctx, payload.URL)
		if err != nil {
			return nil, err
		}
	case payload.Data != "":
		sourceLabel = "inline"
		reader = io.NopCloser(base64.NewDecoder(base64.StdEncoding, strings.NewReader(payload.Data)))
	default:
		return nil, fmt.Errorf("missing image payload")
	}

	if closer := reader; closer != nil {
		defer closer.Close()
	}

	output, err := p.imagePipeline.Process(ctx, domainimage.Input{
		Reader:         reader,
		DeclaredFormat: formatHint,
		Source:         sourceLabel,
	})
	if err != nil {
		return nil, err
	}

	return output, nil
}

func (p *Provider) downloadImage(ctx context.Context, url string) (io.ReadCloser, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "XiaoZhi-Vision/1.0")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch image: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", fmt.Errorf("unexpected status: %s", resp.Status)
	}

	contentType := resp.Header.Get("Content-Type")
	if !p.isValidImageContentType(contentType) {
		resp.Body.Close()
		return nil, "", fmt.Errorf("unsupported content-type: %s", contentType)
	}

	if resp.ContentLength > 0 && p.security.MaxFileSize > 0 && resp.ContentLength > p.security.MaxFileSize {
		resp.Body.Close()
		return nil, "", fmt.Errorf("remote image exceeds max size: %d", resp.ContentLength)
	}

	return resp.Body, inferFormatFromContentType(contentType), nil
}

func (p *Provider) isValidImageContentType(contentType string) bool {
	if contentType == "" {
		return false
	}

	lower := strings.ToLower(contentType)
	validContentTypes := []string{
		"image/jpeg",
		"image/jpg",
		"image/png",
		"image/gif",
		"image/webp",
		"image/bmp",
	}

	for _, valid := range validContentTypes {
		if strings.Contains(lower, valid) {
			return true
		}
	}
	return false
}

func inferFormatFromContentType(contentType string) string {
	lower := strings.ToLower(contentType)
	switch {
	case strings.Contains(lower, "jpeg"), strings.Contains(lower, "jpg"):
		return "jpeg"
	case strings.Contains(lower, "png"):
		return "png"
	case strings.Contains(lower, "gif"):
		return "gif"
	case strings.Contains(lower, "webp"):
		return "webp"
	case strings.Contains(lower, "bmp"):
		return "bmp"
	default:
		return ""
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
			p.logger.Error("Ollama请求序列化失败: %v", err)
			return
		}

		// 发送请求到Ollama
		url := fmt.Sprintf("%s/api/chat", strings.TrimSuffix(p.config.BaseURL, "/"))
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestBody))
		if err != nil {
			responseChan <- fmt.Sprintf("【创建请求失败: %v】", err)
			p.logger.Error("创建Ollama请求失败: %v", err)
			return
		}

		req.Header.Set("Content-Type", "application/json")

		p.logger.Info(
			"向Ollama发送多模态请求: url=%s model=%s text_length=%d",
			url,
			p.config.ModelName,
			len(text),
		)

		resp, err := p.httpClient.Do(req)
		if err != nil {
			responseChan <- fmt.Sprintf("【Ollama API调用失败: %v】", err)
			p.logger.Error("Ollama API调用失败: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			responseChan <- fmt.Sprintf("【Ollama API返回错误: %d】", resp.StatusCode)
			p.logger.Error(
				"Ollama API返回错误: status_code=%d status=%s",
				resp.StatusCode,
				resp.Status,
			)
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
					p.logger.Error("解析Ollama响应失败: %v", err)
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
func (p *Provider) GetImageMetrics() domainimage.Metrics {
	return domainimage.Metrics{}
}

// GetConfig 获取配置信息
func (p *Provider) GetConfig() *Config {
	return p.config
}
