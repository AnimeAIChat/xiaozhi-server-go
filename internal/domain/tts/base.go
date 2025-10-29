package tts

import (
	"fmt"
	"sync"
	"xiaozhi-server-go/internal/domain/tts/inter"
)

// Manager TTS管理器 - 基于 Eino 框架
type Manager struct {
	mu     sync.RWMutex
	tts    interface{} // Eino TTS component
	config inter.TTSConfig
}

// NewManager 创建TTS管理器
func NewManager(config inter.TTSConfig) *Manager {
	return &Manager{
		config: config,
	}
}

// SetTTS 设置 Eino TTS
func (m *Manager) SetTTS(tts interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tts = tts
}

// GetTTS 获取 Eino TTS
func (m *Manager) GetTTS() interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tts
}

// ToTTS 将文本转换为语音
func (m *Manager) ToTTS(text string) (string, error) {
	// TODO: 实现 Eino TTS 调用
	return "", fmt.Errorf("eino TTS integration not implemented yet")
}

// ToTTSWithConfig 使用指定配置转换文本
func (m *Manager) ToTTSWithConfig(text string, config inter.TTSConfig) (string, error) {
	// TODO: 实现 Eino TTS 调用
	return "", fmt.Errorf("eino TTS integration not implemented yet")
}

// Close 关闭TTS资源
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.tts != nil {
		// Eino TTS 的关闭逻辑，如果有的话
		m.tts = nil
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