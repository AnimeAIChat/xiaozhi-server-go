package pool

import (
	"fmt"

	domainmcp "xiaozhi-server-go/internal/domain/mcp"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/internal/core/providers"
	"xiaozhi-server-go/internal/core/providers/asr"
	"xiaozhi-server-go/internal/core/providers/llm"
	"xiaozhi-server-go/internal/core/providers/tts"
	"xiaozhi-server-go/internal/core/providers/vlllm"
	"xiaozhi-server-go/internal/utils"

	// 导入ASR提供者实现以触发注册
	_ "xiaozhi-server-go/internal/core/providers/asr/doubao"
	_ "xiaozhi-server-go/internal/core/providers/asr/deepgram"
	_ "xiaozhi-server-go/internal/core/providers/asr/gosherpa"
	_ "xiaozhi-server-go/internal/core/providers/asr/stepfun"

	// 导入LLM提供者实现以触发注册
	_ "xiaozhi-server-go/internal/core/providers/llm/coze"
	_ "xiaozhi-server-go/internal/core/providers/llm/doubao"
	_ "xiaozhi-server-go/internal/core/providers/llm/ollama"
	_ "xiaozhi-server-go/internal/core/providers/llm/openai"

	// 导入TTS提供者实现以触发注册
	_ "xiaozhi-server-go/internal/core/providers/tts/deepgram"
	_ "xiaozhi-server-go/internal/core/providers/tts/doubao"
	_ "xiaozhi-server-go/internal/core/providers/tts/edge"
	_ "xiaozhi-server-go/internal/core/providers/tts/gosherpa"

	// 导入VLLM提供者实现以触发注册
	_ "xiaozhi-server-go/internal/core/providers/vlllm/openai"
	_ "xiaozhi-server-go/internal/core/providers/vlllm/ollama"
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
	f.logger.InfoTag("资源池", "%s 资源池关闭，销毁资源", f.Name)

	if provider, ok := resource.(providers.Provider); ok {
		return provider.Cleanup()
	}

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
		cfg := f.config.(*config.VLLLMConfig)
		return vlllm.Create(cfg.Type, cfg, f.logger)
	case "mcp":
		cfg := f.config.(*config.Config)
		manager, err := domainmcp.NewFromConfig(cfg, f.logger)
		if err != nil {
			return nil, err
		}
		return manager, nil
	default:
		return nil, fmt.Errorf("未知的提供者类型: %s", f.providerType)
	}
}

// 创建各类型工厂的便利函数
func NewASRFactory(asrType string, config *config.Config, logger *utils.Logger) ResourceFactory {
	if asrCfg, ok := config.ASR[asrType]; ok {
		asrCfgMap, ok := asrCfg.(map[string]interface{})
		if !ok {
			return nil
		}
		return &ProviderFactory{
			providerType: "asr",
			config: &asr.Config{
				Name: asrType,
				Type: asrType,
				Data: asrCfgMap,
			},
			logger: logger,
			params: map[string]interface{}{
				"type":         asrCfgMap["type"],
				"delete_audio": config.Audio.DeleteAudio,
			},
		}
	}
	return nil
}

func NewLLMFactory(llmType string, config *config.Config, logger *utils.Logger) ResourceFactory {
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

func NewTTSFactory(ttsType string, config *config.Config, logger *utils.Logger) ResourceFactory {
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
				"delete_audio": config.Audio.DeleteAudio,
			},
		}
	}
	return nil
}

func NewVLLLMFactory(
	vlllmType string,
	config *config.Config,
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

func NewMCPFactory(config *config.Config, logger *utils.Logger) ResourceFactory {
	return &ProviderFactory{
		providerType: "mcp",
		config:       config,
		logger:       logger,
		params:       map[string]interface{}{},
	}
}
