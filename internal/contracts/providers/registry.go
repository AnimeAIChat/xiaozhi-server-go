package providers

import (
	"errors"
	"sync"
)

// DefaultProviderRegistry 默认提供者注册表实现
type DefaultProviderRegistry struct {
	mu                sync.RWMutex
	asrProviders     map[string]ASRProviderFactory
	llmProviders     map[string]LLMProviderFactory
	ttsProviders     map[string]TTSProviderFactory
}

// NewDefaultProviderRegistry 创建默认提供者注册表
func NewDefaultProviderRegistry() ProviderRegistry {
	return &DefaultProviderRegistry{
		asrProviders: make(map[string]ASRProviderFactory),
		llmProviders: make(map[string]LLMProviderFactory),
		ttsProviders: make(map[string]TTSProviderFactory),
	}
}

// RegisterASRProvider 注册ASR提供者
func (r *DefaultProviderRegistry) RegisterASRProvider(name string, factory ASRProviderFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if factory == nil {
		return errors.New("ASR provider factory cannot be nil")
	}

	if _, exists := r.asrProviders[name]; exists {
		return errors.New("ASR provider already registered: " + name)
	}

	r.asrProviders[name] = factory
	return nil
}

// RegisterLLMProvider 注册LLM提供者
func (r *DefaultProviderRegistry) RegisterLLMProvider(name string, factory LLMProviderFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if factory == nil {
		return errors.New("LLM provider factory cannot be nil")
	}

	if _, exists := r.llmProviders[name]; exists {
		return errors.New("LLM provider already registered: " + name)
	}

	r.llmProviders[name] = factory
	return nil
}

// RegisterTTSProvider 注册TTS提供者
func (r *DefaultProviderRegistry) RegisterTTSProvider(name string, factory TTSProviderFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if factory == nil {
		return errors.New("TTS provider factory cannot be nil")
	}

	if _, exists := r.ttsProviders[name]; exists {
		return errors.New("TTS provider already registered: " + name)
	}

	r.ttsProviders[name] = factory
	return nil
}

// GetASRProvider 获取ASR提供者
func (r *DefaultProviderRegistry) GetASRProvider(name string) (ASRProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.asrProviders[name]
	if !exists {
		return nil, errors.New("ASR provider not found: " + name)
	}

	// 创建提供者实例时需要传入配置，这里返回错误提示需要配置
	return nil, errors.New("use factory.CreateProvider() to create ASR provider instance")
}

// GetLLMProvider 获取LLM提供者
func (r *DefaultProviderRegistry) GetLLMProvider(name string) (LLMProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.llmProviders[name]
	if !exists {
		return nil, errors.New("LLM provider not found: " + name)
	}

	// 创建提供者实例时需要传入配置，这里返回错误提示需要配置
	return nil, errors.New("use factory.CreateProvider() to create LLM provider instance")
}

// GetTTSProvider 获取TTS提供者
func (r *DefaultProviderRegistry) GetTTSProvider(name string) (TTSProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.ttsProviders[name]
	if !exists {
		return nil, errors.New("TTS provider not found: " + name)
	}

	// 创建提供者实例时需要传入配置，这里返回错误提示需要配置
	return nil, errors.New("use factory.CreateProvider() to create TTS provider instance")
}

// ListProviders 列出所有已注册的提供者
func (r *DefaultProviderRegistry) ListProviders() map[string][]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string][]string)

	// 列出ASR提供者
	asrList := make([]string, 0, len(r.asrProviders))
	for name := range r.asrProviders {
		asrList = append(asrList, name)
	}
	result["asr"] = asrList

	// 列出LLM提供者
	llmList := make([]string, 0, len(r.llmProviders))
	for name := range r.llmProviders {
		llmList = append(llmList, name)
	}
	result["llm"] = llmList

	// 列出TTS提供者
	ttsList := make([]string, 0, len(r.ttsProviders))
	for name := range r.ttsProviders {
		ttsList = append(ttsList, name)
	}
	result["tts"] = ttsList

	return result
}