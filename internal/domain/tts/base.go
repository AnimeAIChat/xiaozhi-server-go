package tts

import (
	"fmt"
	"sync"
	"xiaozhi-server-go/internal/domain/tts/inter"
	"xiaozhi-server-go/src/core/providers/tts"
	"xiaozhi-server-go/src/configs"
)

// Manager TTS管理器 - 基于 Eino 框架
type Manager struct {
	mu     sync.RWMutex
	tts    interface{} // Eino TTS component
	config inter.TTSConfig
	globalConfig *configs.Config // 全局配置
	provider inter.TTSProvider // 实际的TTS提供商
}

// NewManager 创建TTS管理器
func NewManager(config inter.TTSConfig, globalConfig *configs.Config) *Manager {
	return &Manager{
		config:       config,
		globalConfig: globalConfig,
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
	return m.ToTTSWithConfig(text, m.config, m.globalConfig)
}

// ToTTSWithConfig 使用指定配置转换文本
func (m *Manager) ToTTSWithConfig(text string, config inter.TTSConfig, globalConfig *configs.Config) (string, error) {
	// 创建TTS配置
	ttsConfig := &tts.Config{
		Type:            config.Provider,
		Voice:           config.Voice,
		Format:          config.Format,
		SampleRate:      config.SampleRate,
		OutputDir:       "tmp/", // 默认输出目录
	}

	// 根据提供商类型设置额外配置
	switch config.Provider {
	case "doubao":
		// 从配置中获取DoubaoTTS的实际配置
		if doubaoCfg, ok := globalConfig.TTS["DoubaoTTS"]; ok {
			ttsConfig.AppID = doubaoCfg.AppID
			ttsConfig.Token = doubaoCfg.Token
			ttsConfig.Cluster = doubaoCfg.Cluster
		} else {
			return "", fmt.Errorf("DoubaoTTS configuration not found")
		}
	case "edge":
		// Edge TTS 不需要额外配置
	}

	// 创建TTS提供商
	provider, err := tts.Create(config.Provider, ttsConfig, true) // deleteFile=true
	if err != nil {
		return "", fmt.Errorf("failed to create TTS provider: %w", err)
	}

	// 初始化提供商
	if err := provider.Initialize(); err != nil {
		return "", fmt.Errorf("failed to initialize TTS provider: %w", err)
	}

	// 设置语音
	if err, _ := provider.SetVoice(config.Voice); err != nil {
		provider.Cleanup()
		return "", fmt.Errorf("failed to set voice: %w", err)
	}

	// 执行TTS转换
	filePath, err := provider.ToTTS(text)
	if err != nil {
		provider.Cleanup()
		return "", fmt.Errorf("failed to convert text to speech: %w", err)
	}

	// 注意：这里不调用Cleanup，因为文件可能还在使用
	// 调用者负责在适当的时候清理文件

	return filePath, nil
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