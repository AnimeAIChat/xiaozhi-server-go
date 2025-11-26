package doubao

import (
	"fmt"

	contractProviders "xiaozhi-server-go/internal/contracts/providers"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/internal/utils"
)

// DoubaoASRFactory Doubao ASR提供者工厂
type DoubaoASRFactory struct {
	providerName string
}

// NewDoubaoASRFactory 创建Doubao ASR工厂
func NewDoubaoASRFactory() contractProviders.ASRProviderFactory {
	return &DoubaoASRFactory{
		providerName: "doubao",
	}
}

// GetProviderName 获取提供者名称
func (f *DoubaoASRFactory) GetProviderName() string {
	return f.providerName
}

// ValidateConfig 验证配置
func (f *DoubaoASRFactory) ValidateConfig(config interface{}) error {
	cfg, ok := config.(Config)
	if !ok {
		return fmt.Errorf("invalid config type, expected doubao.Config")
	}

	if cfg.AppID == "" {
		return fmt.Errorf("app_id is required")
	}

	if cfg.AccessToken == "" {
		return fmt.Errorf("access_token is required")
	}

	if cfg.Host == "" {
		return fmt.Errorf("host is required")
	}

	if cfg.WSURL == "" {
		return fmt.Errorf("ws_url is required")
	}

	return nil
}

// CreateProvider 创建ASR提供者实例
func (f *DoubaoASRFactory) CreateProvider(config interface{}, options map[string]interface{}) (contractProviders.ASRProvider, error) {
	// 解析配置
	var cfg Config

	// 尝试从platform.Config转换为Doubao配置
	if platformConfig, ok := config.(*config.Config); ok {
		cfg = f.extractFromPlatformConfig(platformConfig)
	} else if doubaoConfig, ok := config.(Config); ok {
		cfg = doubaoConfig
	} else {
		return nil, fmt.Errorf("unsupported config type for doubao ASR provider")
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
	provider := NewDoubaoASRProvider(cfg, logger)

	// 初始化提供者
	if err := provider.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize provider: %w", err)
	}

	return provider, nil
}

// extractFromPlatformConfig 从平台配置提取Doubao配置
func (f *DoubaoASRFactory) extractFromPlatformConfig(platformConfig *config.Config) Config {
	return Config{
		AppID:         platformConfig.ASR.Doubao.AppID,
		AccessToken:   platformConfig.ASR.Doubao.AccessToken,
		Host:          platformConfig.ASR.Doubao.Host,
		WSURL:         platformConfig.ASR.Doubao.WsURL,
		ChunkDuration: platformConfig.ASR.Doubao.ChunkDuration,
		ModelName:     platformConfig.ASR.Doubao.Model,
		EndWindowSize: platformConfig.ASR.Doubao.EndWindowSize,
		EnablePunc:    platformConfig.ASR.Doubao.EnablePunc,
		EnableITN:     platformConfig.ASR.Doubao.EnableITN,
		EnableDDC:     platformConfig.ASR.Doubao.EnableDDC,
		OutputDir:     platformConfig.ASR.Doubao.OutputDir,
	}
}