package pool

import (
    "fmt"
    "time"
    "xiaozhi-server-go/src/configs"
    "xiaozhi-server-go/src/core/providers"
    "xiaozhi-server-go/src/core/providers/vlllm"
    "xiaozhi-server-go/src/core/utils"
)

// PoolManager 资源池管理器
type PoolManager struct {
    asrPool   *ResourcePool
    llmPool   *ResourcePool
    ttsPool   *ResourcePool
    vlllmPool *ResourcePool
    logger    *utils.Logger
}

// ProviderSet 提供者集合
type ProviderSet struct {
    ASR   providers.ASRProvider
    LLM   providers.LLMProvider
    TTS   providers.TTSProvider
    VLLLM *vlllm.Provider
}

// NewPoolManager 创建资源池管理器
func NewPoolManager(config *configs.Config, logger *utils.Logger) (*PoolManager, error) {
    pm := &PoolManager{
        logger: logger,
    }

    poolConfig := PoolConfig{
        MinSize:       5,
        MaxSize:       20,
        RefillSize:    3,
        CheckInterval: 30 * time.Second,
    }

    // 检查配置是否包含所需的模块
    selectedModule := config.SelectedModule

    // 初始化ASR池
    if asrType, ok := selectedModule["ASR"]; ok && asrType != "" {
        asrFactory := NewASRFactory(asrType, config, logger)
        if asrFactory == nil {
            return nil, fmt.Errorf("创建ASR工厂失败: 找不到配置 %s", asrType)
        }
        asrPool, err := NewResourcePool(asrFactory, poolConfig, logger)
        if err != nil {
            return nil, fmt.Errorf("初始化ASR资源池失败: %v", err)
        }
        pm.asrPool = asrPool
        _, cnt := asrPool.GetStats()
        logger.FormatInfo("ASR资源池初始化成功，类型: %s, 数量：%d", asrType, cnt)
    }

    // 初始化LLM池
    if llmType, ok := selectedModule["LLM"]; ok && llmType != "" {
        llmFactory := NewLLMFactory(llmType, config, logger)
        if llmFactory == nil {
            return nil, fmt.Errorf("创建LLM工厂失败: 找不到配置 %s", llmType)
        }
        llmPool, err := NewResourcePool(llmFactory, poolConfig, logger)
        if err != nil {
            return nil, fmt.Errorf("初始化LLM资源池失败: %v", err)
        }
        pm.llmPool = llmPool
        _, cnt := llmPool.GetStats()
        logger.FormatInfo("LLM资源池初始化成功，类型: %s, 数量：%d", llmType, cnt)
    }

    // 初始化TTS池
    if ttsType, ok := selectedModule["TTS"]; ok && ttsType != "" {
        ttsFactory := NewTTSFactory(ttsType, config, logger)
        if ttsFactory == nil {
            return nil, fmt.Errorf("创建TTS工厂失败: 找不到配置 %s", ttsType)
        }
        ttsPool, err := NewResourcePool(ttsFactory, poolConfig, logger)
        if err != nil {
            return nil, fmt.Errorf("初始化TTS资源池失败: %v", err)
        }
        pm.ttsPool = ttsPool
        _, cnt := ttsPool.GetStats()
        logger.FormatInfo("TTS资源池初始化成功，类型: %s, 数量：%d", ttsType, cnt)
    }

    // 初始化VLLLM池（可选）
    if vlllmType, ok := selectedModule["VLLLM"]; ok && vlllmType != "" {
        vlllmFactory := NewVLLLMFactory(vlllmType, config, logger)
        if vlllmFactory == nil {
            logger.Warn("创建VLLLM工厂失败: 找不到配置 %s", vlllmType)
        } else {
            vlllmPool, err := NewResourcePool(vlllmFactory, poolConfig, logger)
            if err != nil {
                logger.Warn("初始化VLLLM资源池失败（将继续使用普通LLM）: %v", err)
            } else {
                pm.vlllmPool = vlllmPool
            }
        }
        if pm.vlllmPool != nil {
            _, cnt := pm.vlllmPool.GetStats()
            logger.FormatInfo("VLLLM资源池初始化成功，类型: %s, 数量：%d", vlllmType, cnt)
        } else {
            logger.Warn("VLLLM资源池未初始化，将使用普通LLM")
        }
    }

    return pm, nil
}

// GetProviderSet 获取一套提供者
func (pm *PoolManager) GetProviderSet() (*ProviderSet, error) {
    set := &ProviderSet{}

    if pm.asrPool != nil {
        asr, err := pm.asrPool.Get()
        if err != nil {
            return nil, fmt.Errorf("获取ASR提供者失败: %v", err)
        }
        set.ASR = asr.(providers.ASRProvider)
    }

    if pm.llmPool != nil {
        llm, err := pm.llmPool.Get()
        if err != nil {
            return nil, fmt.Errorf("获取LLM提供者失败: %v", err)
        }
        set.LLM = llm.(providers.LLMProvider)
    }

    if pm.ttsPool != nil {
        tts, err := pm.ttsPool.Get()
        if err != nil {
            return nil, fmt.Errorf("获取TTS提供者失败: %v", err)
        }
        set.TTS = tts.(providers.TTSProvider)
    }

   if pm.vlllmPool != nil {
        vlllmProvider, err := pm.vlllmPool.Get()
        if err == nil {
            // 直接转换，因为我们知道这是从 vlllm 工厂创建的
            set.VLLLM = vlllmProvider.(*vlllm.Provider)
        }
    }

    return set, nil
}

// Close 关闭所有资源池
func (pm *PoolManager) Close() {
    if pm.asrPool != nil {
        pm.asrPool.Close()
    }
    if pm.llmPool != nil {
        pm.llmPool.Close()
    }
    if pm.ttsPool != nil {
        pm.ttsPool.Close()
    }
    if pm.vlllmPool != nil {
        pm.vlllmPool.Close()
    }
}

// GetStats 获取所有池的统计信息
func (pm *PoolManager) GetStats() map[string]map[string]int {
    stats := make(map[string]map[string]int)
    
    if pm.asrPool != nil {
        available, total := pm.asrPool.GetStats()
        stats["asr"] = map[string]int{"available": available, "total": total}
    }
    
    if pm.llmPool != nil {
        available, total := pm.llmPool.GetStats()
        stats["llm"] = map[string]int{"available": available, "total": total}
    }
    
    if pm.ttsPool != nil {
        available, total := pm.ttsPool.GetStats()
        stats["tts"] = map[string]int{"available": available, "total": total}
    }
    
    if pm.vlllmPool != nil {
        available, total := pm.vlllmPool.GetStats()
        stats["vlllm"] = map[string]int{"available": available, "total": total}
    }
    
    return stats
}