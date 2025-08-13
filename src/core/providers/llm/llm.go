package llm

import (
	"fmt"
	"xiaozhi-server-go/src/core/types"
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
	types.LLMProvider
}

// BaseProvider LLM基础实现
type BaseProvider struct {
	config    *Config
	SessionID string // 当前会话ID
}

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

func (p *BaseProvider) SetIdentityFlag(idType string, flag string) {
	// 默认实现，子类可以覆盖
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

	if err := provider.Initialize(); err != nil {
		return nil, fmt.Errorf("初始化LLM提供者失败: %v", err)
	}

	return provider, nil
}
