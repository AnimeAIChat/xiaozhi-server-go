package provider

import (
	"fmt"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/mcp"
	"xiaozhi-server-go/src/core/providers"
	"xiaozhi-server-go/src/core/providers/vlllm"
	"xiaozhi-server-go/src/core/utils"
)

// ProviderManager creates providers for each connection and destroys them
// when that connection closes. No provider instances are shared.
type ProviderManager struct {
	config       *configs.Config
	logger       *utils.Logger
	asrFactory   *ProviderFactory
	llmFactory   *ProviderFactory
	ttsFactory   *ProviderFactory
	vlllmFactory *ProviderFactory
	mcpFactory   *ProviderFactory
}

type ProviderSet struct {
	ASR   providers.ASRProvider
	LLM   providers.LLMProvider
	TTS   providers.TTSProvider
	VLLLM *vlllm.Provider
	MCP   *mcp.Manager
}

func NewProviderManager(config *configs.Config, logger *utils.Logger) (*ProviderManager, error) {
	pm := &ProviderManager{
		config: config,
		logger: logger,
	}

	selectedModule := config.SelectedModule
	if selectedModule == nil {
		selectedModule = map[string]string{}
	}

	if asrType := selectedModule["ASR"]; asrType != "" {
		asrFactory := NewASRFactory(asrType, config, logger)
		if asrFactory == nil {
			return nil, fmt.Errorf("创建ASR工厂失败: 找不到配置 %s", asrType)
		}
		pm.asrFactory = asrFactory
		logger.Info("[ASR] 当前选择的ASR模型: %s", asrType)
	} else {
		logger.Warn("[ASR] 当前选择的ASR模型为空")
	}

	if llmType := selectedModule["LLM"]; llmType != "" {
		llmFactory := NewLLMFactory(llmType, config, logger)
		if llmFactory == nil {
			return nil, fmt.Errorf("创建LLM工厂失败: 找不到配置 %s", llmType)
		}
		pm.llmFactory = llmFactory
	} else {
		logger.Warn("[LLM] 当前选择的LLM模型为空")
	}

	if ttsType := selectedModule["TTS"]; ttsType != "" {
		ttsFactory := NewTTSFactory(ttsType, config, logger)
		if ttsFactory == nil {
			return nil, fmt.Errorf("创建TTS工厂失败: 找不到配置 %s", ttsType)
		}
		pm.ttsFactory = ttsFactory
		logger.Info("[TTS] 当前选择的TTS模型: %s", ttsType)
	} else {
		logger.Warn("[TTS] 当前选择的TTS模型为空")
	}

	if vlllmType := selectedModule["VLLLM"]; vlllmType != "" {
		vlllmFactory := NewVLLLMFactory(vlllmType, config, logger)
		if vlllmFactory == nil {
			logger.Warn("创建VLLLM工厂失败: 找不到配置 %s", vlllmType)
		} else {
			pm.vlllmFactory = vlllmFactory
			logger.Info("[VLLLM] 当前选择的VLLLM模型: %s", vlllmType)
		}
	}

	pm.mcpFactory = NewMCPFactory(config, logger)
	return pm, nil
}

func (pm *ProviderManager) GetProviderSet() (*ProviderSet, error) {
	set := &ProviderSet{}

	if pm.asrFactory != nil {
		asr, err := pm.asrFactory.Create()
		if err != nil {
			pm.DestroyProviderSet(set)
			return nil, fmt.Errorf("创建ASR提供者失败: %v", err)
		}
		set.ASR = asr.(providers.ASRProvider)
	}

	if pm.llmFactory != nil {
		llm, err := pm.llmFactory.Create()
		if err != nil {
			pm.DestroyProviderSet(set)
			return nil, fmt.Errorf("创建LLM提供者失败: %v", err)
		}
		set.LLM = llm.(providers.LLMProvider)
	}

	if pm.ttsFactory != nil {
		tts, err := pm.ttsFactory.Create()
		if err != nil {
			pm.DestroyProviderSet(set)
			return nil, fmt.Errorf("创建TTS提供者失败: %v", err)
		}
		set.TTS = tts.(providers.TTSProvider)
	}

	if pm.vlllmFactory != nil {
		vlllmProvider, err := pm.vlllmFactory.Create()
		if err != nil {
			pm.logger.Warn("创建VLLLM提供者失败，将继续使用普通LLM: %v", err)
		} else {
			set.VLLLM = vlllmProvider.(*vlllm.Provider)
		}
	}

	if pm.mcpFactory != nil {
		mcpManager, err := pm.mcpFactory.Create()
		if err != nil {
			pm.DestroyProviderSet(set)
			return nil, fmt.Errorf("创建MCP管理器失败: %v", err)
		}
		set.MCP = mcpManager.(*mcp.Manager)
	}

	return set, nil
}

func (pm *ProviderManager) ReturnProviderSet(set *ProviderSet) error {
	return pm.DestroyProviderSet(set)
}

func (pm *ProviderManager) DestroyProviderSet(set *ProviderSet) error {
	if set == nil {
		return nil
	}

	var errs []error
	cleanup := func(name string, resource interface{}) {
		if resource == nil {
			return
		}
		if cleaner, ok := resource.(interface{ Cleanup() error }); ok {
			if err := cleaner.Cleanup(); err != nil {
				errs = append(errs, fmt.Errorf("%s cleanup failed: %v", name, err))
			}
		}
	}

	cleanup("ASR", set.ASR)
	cleanup("LLM", set.LLM)
	cleanup("TTS", set.TTS)
	cleanup("VLLLM", set.VLLLM)
	cleanup("MCP", set.MCP)

	if len(errs) > 0 {
		return fmt.Errorf("销毁提供者时发生错误: %v", errs)
	}
	return nil
}

func (pm *ProviderManager) Close() {}
