package llm

import (
	"fmt"
	"xiaozhi-server-go/internal/core/providers"
	"xiaozhi-server-go/internal/domain/eventbus"
)

// Config LLM配置结构
type Config struct {
	Name        string                 `yaml:"name"` // LLM提供者名称
	Type        string                 `yaml:"type"`
	ModelName   string                 `yaml:"model_name"`
	BaseURL     string                 `yaml:"base_url,omitempty"`
	APIKey      string                 `yaml:"api_key,omitempty"`
	Temperature float64                `yaml:"temperature,omitempty"`
	MaxTokens   int                    `yaml:"max_tokens,omitempty"`
	TopP        float64                `yaml:"top_p,omitempty"`
	Extra       map[string]interface{} `yaml:",inline"`
}

// Provider LLM提供者接口
type Provider interface {
	providers.LLMProvider
}

// BaseProvider LLM基础实现
type BaseProvider struct {
	config    *Config
	SessionID string // 当前会话ID
}

// BaseProvider 实现 EventPublisher 接口
var _ EventPublisher = (*BaseProvider)(nil)

// Config 获取配置
func (p *BaseProvider) Config() *Config {
	return p.config
}

// NewBaseProvider 创建LLM基础提供者
func NewBaseProvider(config *Config) *BaseProvider {
	return &BaseProvider{
		config: config,
	}
}

// Initialize 初始化提供者
func (p *BaseProvider) Initialize() error {
	return nil
}

// Cleanup 清理资源
func (p *BaseProvider) Cleanup() error {
	return nil
}

func (p *BaseProvider) GetSessionID() string {
	return p.SessionID
}

func (p *BaseProvider) SetSessionID(sessionID string) {
	p.SessionID = sessionID
}

// PublishLLMResponse 发布LLM回复事件
func (p *BaseProvider) PublishLLMResponse(content string, isFinal bool, round int, toolCalls interface{}, textIndex int, spentTime string) {
	eventData := eventbus.LLMEventData{
		SessionID: p.SessionID,
		Round:     round,
		Content:   content,
		IsFinal:   isFinal,
		TextIndex: textIndex,
		SpentTime: spentTime,
		ToolCalls: toolCalls,
	}
	eventbus.Publish(eventbus.EventLLMResponse, eventData)
}

// PublishLLMError 发布LLM错误事件
func (p *BaseProvider) PublishLLMError(err error, round int) {
	eventData := eventbus.SystemEventData{
		Level:   "error",
		Message: fmt.Sprintf("LLM error: %v", err),
		Data: map[string]interface{}{
			"session_id": p.SessionID,
			"round":      round,
			"error":      err.Error(),
		},
	}
	eventbus.Publish(eventbus.EventLLMError, eventData)
}

func (p *BaseProvider) SetIdentityFlag(idType string, flag string) {
	// 默认实现，子类可以覆盖
}

// EventPublisher 事件发布接口
type EventPublisher interface {
	SetSessionID(sessionID string)
	PublishLLMResponse(content string, isFinal bool, round int, toolCalls interface{}, textIndex int, spentTime string)
	PublishLLMError(err error, round int)
}

// GetEventPublisher 获取事件发布器
func GetEventPublisher(provider Provider) EventPublisher {
	if p, ok := provider.(EventPublisher); ok {
		return p
	}
	return nil
}

// Factory LLM工厂函数类型
type Factory func(config *Config) (Provider, error)

var factories = make(map[string]Factory)

// Register 注册LLM提供者工厂
func Register(name string, factory Factory) {
	factories[name] = factory
}

// Create 创建LLM提供者实例
func Create(name string, config *Config) (Provider, error) {
	factory, ok := factories[name]
	if !ok {
		return nil, fmt.Errorf("未知的LLM提供者: %s", name)
	}

	provider, err := factory(config)
	if err != nil {
		return nil, fmt.Errorf("创建LLM提供者失败: %v", err)
	}

	return provider, nil
}
