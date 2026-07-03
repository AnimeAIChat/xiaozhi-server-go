package provider

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

// ProviderFactory creates one provider instance for one connection.
type ProviderFactory struct {
	Name         string
	providerType string
	config       interface{}
	logger       *utils.Logger
	params       map[string]interface{}
}

func (f *ProviderFactory) Create() (interface{}, error) {
	return f.createProvider()
}

func (f *ProviderFactory) Destroy(resource interface{}) error {
	if provider, ok := resource.(providers.Provider); ok {
		return provider.Cleanup()
	}
	if cleaner, ok := resource.(interface{ Cleanup() error }); ok {
		return cleaner.Cleanup()
	}
	return nil
}

func (f *ProviderFactory) createProvider() (interface{}, error) {
	switch f.providerType {
	case "asr":
		cfg := f.config.(*asr.Config)
		deleteAudio, _ := f.params["delete_audio"].(bool)
		asrType, _ := f.params["type"].(string)
		return asr.Create(asrType, cfg, deleteAudio, f.logger)
	case "llm":
		cfg := f.config.(*llm.Config)
		return llm.Create(cfg.Type, cfg)
	case "tts":
		cfg := f.config.(*tts.Config)
		deleteAudio, _ := f.params["delete_audio"].(bool)
		return tts.Create(cfg.Type, cfg, deleteAudio)
	case "vlllm":
		cfg := f.config.(*configs.VLLMConfig)
		return vlllm.Create(cfg.Type, cfg, f.logger)
	case "mcp":
		cfg := f.config.(*configs.Config)
		return mcp.NewManager(f.logger, cfg), nil
	default:
		return nil, fmt.Errorf("unknown provider type: %s", f.providerType)
	}
}

func NewASRFactory(asrType string, config *configs.Config, logger *utils.Logger) *ProviderFactory {
	if asrCfg, ok := config.ASR[asrType]; ok {
		return &ProviderFactory{
			Name:         asrType,
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

func NewLLMFactory(llmType string, config *configs.Config, logger *utils.Logger) *ProviderFactory {
	if llmCfg, ok := config.LLM[llmType]; ok {
		logger.Info("[LLM] 当前选择的LLM模型: %s type=%s model=%s url=%s", llmType, llmCfg.Type, llmCfg.ModelName, llmCfg.BaseURL)
		return &ProviderFactory{
			Name:         llmType,
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

func NewTTSFactory(ttsType string, config *configs.Config, logger *utils.Logger) *ProviderFactory {
	if ttsCfg, ok := config.TTS[ttsType]; ok {
		return &ProviderFactory{
			Name:         ttsType,
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
				"delete_audio": config.DeleteAudio,
			},
		}
	}
	return nil
}

func NewVLLLMFactory(vlllmType string, config *configs.Config, logger *utils.Logger) *ProviderFactory {
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

func NewMCPFactory(config *configs.Config, logger *utils.Logger) *ProviderFactory {
	return &ProviderFactory{
		Name:         "mcp",
		providerType: "mcp",
		config:       config,
		logger:       logger,
		params:       map[string]interface{}{},
	}
}
