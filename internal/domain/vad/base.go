package vad

import (
	"fmt"
	"sync"
	"xiaozhi-server-go/internal/domain/vad/inter"
)

// Manager VAD管理器
type Manager struct {
	mu       sync.RWMutex
	provider inter.VADProvider
	config   inter.VADConfig
}

// NewManager 创建VAD管理器
func NewManager(config inter.VADConfig) *Manager {
	return &Manager{
		config: config,
	}
}

// SetProvider 设置VAD提供者
func (m *Manager) SetProvider(provider inter.VADProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.provider = provider
}

// GetProvider 获取VAD提供者
func (m *Manager) GetProvider() inter.VADProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.provider
}

// ProcessAudio 处理音频数据
func (m *Manager) ProcessAudio(audioData []byte) (bool, error) {
	m.mu.RLock()
	provider := m.provider
	m.mu.RUnlock()

	if provider == nil {
		return false, fmt.Errorf("VAD provider not set")
	}

	return provider.ProcessAudio(audioData)
}

// Reset 重置VAD状态
func (m *Manager) Reset() {
	m.mu.RLock()
	provider := m.provider
	m.mu.RUnlock()

	if provider != nil {
		provider.Reset()
	}
}

// Close 关闭VAD资源
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
func (m *Manager) GetConfig() inter.VADConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// UpdateConfig 更新配置
func (m *Manager) UpdateConfig(config inter.VADConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
}

// ValidateConfig 验证配置
func ValidateConfig(config inter.VADConfig) error {
	if config.SampleRate <= 0 {
		return fmt.Errorf("invalid sample rate: %d", config.SampleRate)
	}
	if config.Channels <= 0 {
		return fmt.Errorf("invalid channels: %d", config.Channels)
	}
	if config.FrameDuration <= 0 {
		return fmt.Errorf("invalid frame duration: %d", config.FrameDuration)
	}
	if config.Sensitivity < 0 || config.Sensitivity > 1 {
		return fmt.Errorf("invalid sensitivity: %f", config.Sensitivity)
	}
	return nil
}

// DefaultConfig 获取默认配置
func DefaultConfig() inter.VADConfig {
	return inter.VADConfig{
		SampleRate:      16000,
		Channels:        1,
		FrameDuration:   30,
		Sensitivity:     0.5,
		MinSpeechLength: 100,
		MaxSilenceLength: 500,
	}
}

// AcquireVAD 获取VAD实例（向后兼容）
func AcquireVAD(provider string, config map[string]interface{}) (inter.VADProvider, error) {
	// TODO: 实现VAD工厂方法，避免循环导入
	// 暂时返回错误，后续通过依赖注入或其他方式解决
	return nil, fmt.Errorf("VAD provider %s not implemented yet (working on import cycle issue)", provider)
}

// ReleaseVAD 释放VAD实例（向后兼容）
func ReleaseVAD(vad inter.VADProvider) error {
	if vad != nil {
		return vad.Close()
	}
	return nil
}