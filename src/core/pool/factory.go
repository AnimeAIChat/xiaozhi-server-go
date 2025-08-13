package pool

import (
	"fmt"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/mcp"
	"xiaozhi-server-go/src/core/providers"
	"xiaozhi-server-go/src/core/providers/asr"
	"xiaozhi-server-go/src/core/providers/llm"
	"xiaozhi-server-go/src/core/providers/tts"
	"xiaozhi-server-go/src/core/providers/vlllm"
	"xiaozhi-server-go/src/core/utils"
)

/*
* 工厂类，用于创建不同类型的资源池工厂。
* 通过配置文件和提供者类型，动态创建资源池工厂。
* 支持ASR、LLM、TTS和VLLLM等多种提供者类型。
* 每个工厂实现了ResourceFactory接口，提供Create和Destroy方法。
 */

// ProviderFactory 简化的提供者工厂
type ProviderFactory struct {
	Name         string // 提供者名称
	providerType string
	config       interface{}
	logger       *utils.Logger
	params       map[string]interface{} // 可选参数
}

func (f *ProviderFactory) Create() (interface{}, error) {
	return f.createProvider()
}

func (f *ProviderFactory) Destroy(resource interface{}) error {
	if provider, ok := resource.(providers.Provider); ok {
		return provider.Cleanup()
	}
	// 对于VLLLM，我们尝试调用Cleanup方法（如果存在）
	if resource != nil {
		// 使用反射或类型断言来调用Cleanup方法
		if cleaner, ok := resource.(interface{ Cleanup() error }); ok {
			return cleaner.Cleanup()
		}
	}
	return nil
}

func (f *ProviderFactory) createProvider() (interface{}, error) {
	switch f.providerType {
	case "asr":
		cfg := f.config.(*asr.Config)
		params := f.params
		delete_audio, _ := params["delete_audio"].(bool)
		asrType, _ := params["type"].(string)
		return asr.Create(asrType, cfg, delete_audio, f.logger)
	case "llm":
		cfg := f.config.(*llm.Config)
		return llm.Create(cfg.Type, cfg)
	case "tts":
		cfg := f.config.(*tts.Config)
		params := f.params
		delete_audio, _ := params["delete_audio"].(bool)
		return tts.Create(cfg.Type, cfg, delete_audio)
	case "vlllm":
		cfg := f.config.(*configs.VLLMConfig)
		return vlllm.Create(cfg.Type, cfg, f.logger)
	case "mcp":
		cfg := f.config.(*configs.Config)
		logger := f.logger
		return mcp.NewManagerForPool(logger, cfg), nil
	default:
		return nil, fmt.Errorf("未知的提供者类型: %s", f.providerType)
	}
}

// 创建各类型工厂的便利函数
func NewASRFactory(asrType string, config *configs.Config, logger *utils.Logger) ResourceFactory {
	if asrCfg, ok := config.ASR[asrType]; ok {
		return &ProviderFactory{
			providerType: "asr",
			config: &asr.Config{
				Name: asrType,
				Type: asrType,
				Data: asrCfg,
			},
			logger: logger,
			params: map[string]interface{}{
				"type":         asrCfg["type"],
				"delete_audio": config.DeleteAudio,
			},
		}
	}
	return nil
}

func NewLLMFactory(llmType string, config *configs.Config, logger *utils.Logger) ResourceFactory {
	if llmCfg, ok := config.LLM[llmType]; ok {
		return &ProviderFactory{
			providerType: "llm",
			config: &llm.Config{
				Name:        llmType,
				Type:        llmCfg.Type,
				ModelName:   llmCfg.ModelName,
				BaseURL:     llmCfg.BaseURL,
				APIKey:      llmCfg.APIKey,
				Temperature: llmCfg.Temperature,
				MaxTokens:   llmCfg.MaxTokens,
				TopP:        llmCfg.TopP,
				Extra:       llmCfg.Extra,
			},
			logger: logger,
		}
	}
	return nil
}

func NewTTSFactory(ttsType string, config *configs.Config, logger *utils.Logger) ResourceFactory {
	if ttsCfg, ok := config.TTS[ttsType]; ok {
		return &ProviderFactory{
			providerType: "tts",
			config: &tts.Config{
				Name:            ttsType,
				Type:            ttsCfg.Type,
				Voice:           ttsCfg.Voice,
				Format:          ttsCfg.Format,
				OutputDir:       ttsCfg.OutputDir,
				AppID:           ttsCfg.AppID,
				Token:           ttsCfg.Token,
				Cluster:         ttsCfg.Cluster,
				SupportedVoices: ttsCfg.SupportedVoices,
			},
			logger: logger,
			params: map[string]interface{}{
				"type":         ttsCfg.Type,
				"delete_audio": config.DeleteAudio,
			},
		}
	}
	return nil
}

func NewVLLLMFactory(
	vlllmType string,
	config *configs.Config,
	logger *utils.Logger,
) ResourceFactory {
	if vlllmCfg, ok := config.VLLLM[vlllmType]; ok {
		return &ProviderFactory{
			Name:         vlllmType,
			providerType: "vlllm",
			config:       &vlllmCfg,
			logger:       logger,
		}
	}
	return nil
}

func NewMCPFactory(config *configs.Config, logger *utils.Logger) ResourceFactory {
	return &ProviderFactory{
		providerType: "mcp",
		config:       config,
		logger:       logger,
		params:       map[string]interface{}{},
	}
}
