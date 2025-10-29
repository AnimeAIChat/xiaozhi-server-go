package asr

import (
	"fmt"
	"sync"
	"xiaozhi-server-go/internal/domain/asr/inter"
)

// Manager ASR管理器
type Manager struct {
	mu       sync.RWMutex
	provider inter.ASRProvider
	config   inter.ASRConfig
}

// NewManager 创建ASR管理器
func NewManager(config inter.ASRConfig) *Manager {
	return &Manager{
		config: config,
	}
}

// SetProvider 设置ASR提供者
func (m *Manager) SetProvider(provider inter.ASRProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.provider = provider
}

// GetProvider 获取ASR提供者
func (m *Manager) GetProvider() inter.ASRProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.provider
}

// StartListening 开始监听
func (m *Manager) StartListening() error {
	m.mu.RLock()
	provider := m.provider
	m.mu.RUnlock()

	if provider == nil {
		return fmt.Errorf("ASR provider not set")
	}

	return provider.StartListening()
}

// StopListening 停止监听
func (m *Manager) StopListening() error {
	m.mu.RLock()
	provider := m.provider
	m.mu.RUnlock()

	if provider == nil {
		return fmt.Errorf("ASR provider not set")
	}

	return provider.StopListening()
}

// ProcessAudioData 处理音频数据
func (m *Manager) ProcessAudioData(audioData []byte) error {
	m.mu.RLock()
	provider := m.provider
	m.mu.RUnlock()

	if provider == nil {
		return fmt.Errorf("ASR provider not set")
	}

	return provider.ProcessAudioData(audioData)
}

// SetEventListener 设置事件监听器
func (m *Manager) SetEventListener(listener inter.ASREventListener) error {
	m.mu.RLock()
	provider := m.provider
	m.mu.RUnlock()

	if provider == nil {
		return fmt.Errorf("ASR provider not set")
	}

	return provider.SetEventListener(listener)
}

// Reset 重置ASR状态
func (m *Manager) Reset() {
	m.mu.RLock()
	provider := m.provider
	m.mu.RUnlock()

	if provider != nil {
		provider.Reset()
	}
}

// Close 关闭ASR资源
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.provider != nil {
		err := m.provider.Close()
		m.provider = nil
		return err
	}
	return nil
}

// GetConfig 获取配置
func (m *Manager) GetConfig() inter.ASRConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// UpdateConfig 更新配置
func (m *Manager) UpdateConfig(config inter.ASRConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
}

// ValidateConfig 验证配置
func ValidateConfig(config inter.ASRConfig) error {
	if config.SampleRate <= 0 {
		return fmt.Errorf("invalid sample rate: %d", config.SampleRate)
	}
	if config.Channels <= 0 {
		return fmt.Errorf("invalid channels: %d", config.Channels)
	}
	if config.SilenceTimeout < 0 {
		return fmt.Errorf("invalid silence timeout: %d", config.SilenceTimeout)
	}
	if config.Sensitivity < 0 || config.Sensitivity > 1 {
		return fmt.Errorf("invalid sensitivity: %f", config.Sensitivity)
	}
	return nil
}

// DefaultConfig 获取默认配置
func DefaultConfig() inter.ASRConfig {
	return inter.ASRConfig{
		Provider:       "funasr",
		SampleRate:     16000,
		Channels:       1,
		Language:       "zh-CN",
		SilenceTimeout: 2000,
		MaxSpeechLength: 30000,
		Sensitivity:    0.5,
	}
}