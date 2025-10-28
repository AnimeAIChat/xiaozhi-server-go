package tts

import (
	"fmt"
	"sync"
	"xiaozhi-server-go/internal/domain/tts/inter"
)

// Manager TTS管理器
type Manager struct {
	mu       sync.RWMutex
	provider inter.TTSProvider
	config   inter.TTSConfig
}

// NewManager 创建TTS管理器
func NewManager(config inter.TTSConfig) *Manager {
	return &Manager{
		config: config,
	}
}

// SetProvider 设置TTS提供者
func (m *Manager) SetProvider(provider inter.TTSProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.provider = provider
}

// GetProvider 获取TTS提供者
func (m *Manager) GetProvider() inter.TTSProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.provider
}

// ToTTS 将文本转换为语音
func (m *Manager) ToTTS(text string) (string, error) {
	m.mu.RLock()
	provider := m.provider
	m.mu.RUnlock()

	if provider == nil {
		return "", fmt.Errorf("TTS provider not set")
	}

	return provider.ToTTS(text)
}

// ToTTSWithConfig 使用指定配置转换文本
func (m *Manager) ToTTSWithConfig(text string, config inter.TTSConfig) (string, error) {
	m.mu.RLock()
	provider := m.provider
	m.mu.RUnlock()

	if provider == nil {
		return "", fmt.Errorf("TTS provider not set")
	}

	return provider.ToTTSWithConfig(text, config)
}

// Close 关闭TTS资源
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
func (m *Manager) GetConfig() inter.TTSConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// UpdateConfig 更新配置
func (m *Manager) UpdateConfig(config inter.TTSConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
}

// ValidateConfig 验证配置
func ValidateConfig(config inter.TTSConfig) error {
	if config.Provider == "" {
		return fmt.Errorf("provider cannot be empty")
	}
	if config.Voice == "" {
		return fmt.Errorf("voice cannot be empty")
	}
	if config.Speed <= 0 || config.Speed > 3 {
		return fmt.Errorf("invalid speed: %f", config.Speed)
	}
	if config.Pitch <= 0 || config.Pitch > 3 {
		return fmt.Errorf("invalid pitch: %f", config.Pitch)
	}
	if config.Volume < 0 || config.Volume > 1 {
		return fmt.Errorf("invalid volume: %f", config.Volume)
	}
	if config.SampleRate <= 0 {
		return fmt.Errorf("invalid sample rate: %d", config.SampleRate)
	}
	return nil
}

// DefaultConfig 获取默认配置
func DefaultConfig() inter.TTSConfig {
	return inter.TTSConfig{
		Provider:   "doubao",
		Voice:      "BV001_streaming",
		Speed:      1.0,
		Pitch:      1.0,
		Volume:     1.0,
		SampleRate: 24000,
		Format:     "opus",
		Language:   "zh-CN",
	}
}