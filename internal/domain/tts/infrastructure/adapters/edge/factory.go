package edge

import (
	"fmt"

	contractProviders "xiaozhi-server-go/internal/contracts/providers"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/src/core/utils"
)

// EdgeTTSFactory Edge TTS提供者工厂
type EdgeTTSFactory struct {
	providerName string
}

// NewEdgeTTSFactory 创建Edge TTS工厂
func NewEdgeTTSFactory() contractProviders.TTSProviderFactory {
	return &EdgeTTSFactory{
		providerName: "edge",
	}
}

// GetProviderName 获取提供者名称
func (f *EdgeTTSFactory) GetProviderName() string {
	return f.providerName
}

// ValidateConfig 验证配置
func (f *EdgeTTSFactory) ValidateConfig(config interface{}) error {
	cfg, ok := config.(Config)
	if !ok {
		return fmt.Errorf("invalid config type, expected edge.Config")
	}

	// 验证采样率
	if cfg.SampleRate < 8000 || cfg.SampleRate > 48000 {
		return fmt.Errorf("sample_rate must be between 8000 and 48000")
	}

	// 验证速度参数
	if cfg.Speed < 0.25 || cfg.Speed > 3.0 {
		return fmt.Errorf("speed must be between 0.25 and 3.0")
	}

	// 验证音调参数
	if cfg.Pitch < -20.0 || cfg.Pitch > 20.0 {
		return fmt.Errorf("pitch must be between -20.0 and 20.0")
	}

	// 验证音量参数
	if cfg.Volume < 0.0 || cfg.Volume > 1.0 {
		return fmt.Errorf("volume must be between 0.0 and 1.0")
	}

	return nil
}

// CreateProvider 创建TTS提供者实例
func (f *EdgeTTSFactory) CreateProvider(config interface{}, options map[string]interface{}) (contractProviders.TTSProvider, error) {
	// 解析配置
	var cfg Config

	// 尝试从platform.Config转换为Edge配置
	if platformConfig, ok := config.(*config.Config); ok {
		cfg = f.extractFromPlatformConfig(platformConfig)
	} else if edgeConfig, ok := config.(Config); ok {
		cfg = edgeConfig
	} else {
		return nil, fmt.Errorf("unsupported config type for edge TTS provider")
	}

	// 验证配置
	if err := f.ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	// 获取日志记录器
	var logger *utils.Logger
	if loggerVal, ok := options["logger"]; ok {
		if logger, ok = loggerVal.(*utils.Logger); !ok {
			return nil, fmt.Errorf("logger option must be *utils.Logger")
		}
	} else {
		logger = utils.DefaultLogger
	}

	// 创建提供者实例
	provider := NewEdgeTTSProvider(cfg, logger)

	// 初始化提供者
	if err := provider.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize provider: %w", err)
	}

	return provider, nil
}

// extractFromPlatformConfig 从平台配置提取Edge配置
func (f *EdgeTTSFactory) extractFromPlatformConfig(platformConfig *config.Config) Config {
	return Config{
		Voice:      platformConfig.TTS.Edge.Voice,
		OutputDir:  platformConfig.TTS.Edge.OutputDir,
		DeleteFile: platformConfig.TTS.Edge.DeleteFile,
		SampleRate: platformConfig.TTS.Edge.SampleRate,
		Format:     platformConfig.TTS.Edge.Format,
		Speed:      platformConfig.TTS.Edge.Speed,
		Pitch:      platformConfig.TTS.Edge.Pitch,
		Volume:     platformConfig.TTS.Edge.Volume,
	}
}