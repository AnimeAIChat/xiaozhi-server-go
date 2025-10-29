package vad

import (
	"fmt"
	"xiaozhi-server-go/internal/domain/vad/inter"
)

// VADProvider VAD提供者接口
type VADProvider interface {
	// ProcessAudio 处理音频数据，返回是否检测到语音活动
	ProcessAudio(audioData []byte) (bool, error)

	// Reset 重置VAD状态
	Reset()

	// Close 关闭VAD资源
	Close() error

	// GetConfig 获取VAD配置
	GetConfig() inter.VADConfig
}

// Config VAD配置结构
type Config struct {
	Name        string                 `yaml:"name"` // VAD提供者名称
	Type        string                 `yaml:"type"`
	SampleRate  int                    `yaml:"sample_rate"`
	Channels    int                    `yaml:"channels"`
	Extra       map[string]interface{} `yaml:",inline"`
}

// Provider VAD提供者接口
type Provider interface {
	VADProvider
}

// BaseProvider VAD基础实现
type BaseProvider struct {
	config *Config
}

// Config 获取配置
func (p *BaseProvider) Config() *Config {
	return p.config
}

// NewBaseProvider 创建VAD基础提供者
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

// Factory VAD工厂函数类型
type Factory func(config *Config) (Provider, error)

var factories = make(map[string]Factory)

// Register 注册VAD提供者工厂
func Register(name string, factory Factory) {
	factories[name] = factory
}

// Create 创建VAD提供者实例
func Create(name string, config *Config) (Provider, error) {
	factory, ok := factories[name]
	if !ok {
		return nil, fmt.Errorf("未知的VAD提供者: %s", name)
	}

	provider, err := factory(config)
	if err != nil {
		return nil, fmt.Errorf("创建VAD提供者失败: %v", err)
	}

	return provider, nil
}

// GetVADProvider 获取VAD提供者（适配器）
func GetVADProvider(provider Provider) inter.VADProvider {
	if p, ok := provider.(inter.VADProvider); ok {
		return p
	}
	return nil
}