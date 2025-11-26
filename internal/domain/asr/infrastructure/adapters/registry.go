package adapters

import (
	"fmt"

	contractProviders "xiaozhi-server-go/internal/contracts/providers"
	"xiaozhi-server-go/internal/domain/asr/infrastructure/adapters/doubao"
)

// ASRRegistry ASR提供者注册器
type ASRRegistry struct {
	factories map[string]contractProviders.ASRProviderFactory
}

// NewASRRegistry 创建ASR注册器
func NewASRRegistry() *ASRRegistry {
	registry := &ASRRegistry{
		factories: make(map[string]contractProviders.ASRProviderFactory),
	}

	// 注册内置的ASR提供者
	registry.registerBuiltinProviders()

	return registry
}

// registerBuiltinProviders 注册内置的ASR提供者
func (r *ASRRegistry) registerBuiltinProviders() {
	// 注册Doubao ASR提供者
	doubaoFactory := doubao.NewDoubaoASRFactory()
	r.factories[doubaoFactory.GetProviderName()] = doubaoFactory

	// TODO: 在后续阶段注册其他ASR提供者
	// deepgramFactory := deepgram.NewDeepgramASRFactory()
	// r.factories[deepgramFactory.GetProviderName()] = deepgramFactory
}

// RegisterFactory 注册ASR提供者工厂
func (r *ASRRegistry) RegisterFactory(name string, factory contractProviders.ASRProviderFactory) error {
	if factory == nil {
		return fmt.Errorf("factory cannot be nil")
	}

	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("ASR provider factory '%s' already registered", name)
	}

	r.factories[name] = factory
	return nil
}

// GetFactory 获取ASR提供者工厂
func (r *ASRRegistry) GetFactory(name string) (contractProviders.ASRProviderFactory, error) {
	factory, exists := r.factories[name]
	if !exists {
		return nil, fmt.Errorf("ASR provider factory '%s' not found", name)
	}

	return factory, nil
}

// ListFactories 列出所有已注册的工厂
func (r *ASRRegistry) ListFactories() []string {
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	return names
}

// CreateProvider 创建ASR提供者实例
func (r *ASRRegistry) CreateProvider(name string, config interface{}, options map[string]interface{}) (contractProviders.ASRProvider, error) {
	factory, err := r.GetFactory(name)
	if err != nil {
		return nil, err
	}

	return factory.CreateProvider(config, options)
}