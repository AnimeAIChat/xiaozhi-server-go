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

// AcquireVAD 获取VAD实例
func AcquireVAD(provider string, config map[string]interface{}) (inter.VADProvider, error) {
	switch provider {
	case "webrtc":
		// 动态导入避免循环依赖
		// 这里使用类型断言或反射，但为了简单，我们直接调用
		// 实际上这会导致循环导入，需要重构
		return acquireWebRTCVAD(config)
	case "silero":
		return acquireSileroVAD(config)
	default:
		return nil, fmt.Errorf("unsupported VAD provider: %s", provider)
	}
}

// acquireWebRTCVAD 获取WebRTC VAD实例
func acquireWebRTCVAD(config map[string]interface{}) (inter.VADProvider, error) {
	// 为了避免循环导入，这里复制配置逻辑
	vadConfig := inter.VADConfig{
		SampleRate:      16000,
		Channels:        1,
		FrameDuration:   20,
		Sensitivity:     0.5,
		MinSpeechLength: 100,
		MaxSilenceLength: 500,
	}

	// 从config中读取配置
	if sampleRate, ok := config["sample_rate"].(float64); ok {
		vadConfig.SampleRate = int(sampleRate)
	}
	if channels, ok := config["channels"].(float64); ok {
		vadConfig.Channels = int(channels)
	}
	if minSpeechLength, ok := config["min_speech_length"].(float64); ok {
		vadConfig.MinSpeechLength = int(minSpeechLength)
	}
	if maxSilenceLength, ok := config["max_silence_length"].(float64); ok {
		vadConfig.MaxSilenceLength = int(maxSilenceLength)
	}

	// 使用池化
	if poolSize, ok := config["pool_size"].(float64); ok && poolSize > 0 {
		config["pool_max_size"] = poolSize
		config["pool_min_size"] = float64(1)
		config["pool_max_idle_time"] = float64(300) // 5分钟
	}

	// 这里需要调用 webrtc_vad.AcquireVAD，但会有循环导入
	// 暂时返回错误，需要重构架构
	return nil, fmt.Errorf("WebRTC VAD integration needs architecture refactoring to avoid import cycles")
}

// acquireSileroVAD 获取Silero VAD实例
func acquireSileroVAD(config map[string]interface{}) (inter.VADProvider, error) {
	// 类似实现
	return nil, fmt.Errorf("Silero VAD not implemented yet")
}

// ReleaseVAD 释放VAD实例（向后兼容）
func ReleaseVAD(vad inter.VADProvider) error {
	if vad != nil {
		return vad.Close()
	}
	return nil
}