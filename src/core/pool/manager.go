package pool

import (
	"context"
	"fmt"
	"time"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/mcp"
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
	mcpPool   *ResourcePool
	logger    *utils.Logger
}

// ProviderSet 提供者集合
type ProviderSet struct {
	ASR   providers.ASRProvider
	LLM   providers.LLMProvider
	TTS   providers.TTSProvider
	VLLLM *vlllm.Provider
	MCP   *mcp.Manager
}

// NewPoolManager 创建资源池管理器
func NewPoolManager(config *configs.Config, logger *utils.Logger) (*PoolManager, error) {
	pm := &PoolManager{
		logger: logger,
	}

	// 执行连通性检查
	if err := pm.performConnectivityCheck(config, logger); err != nil {
		return nil, fmt.Errorf("资源连通性检查失败: %v", err)
	}

	interval := config.PoolConfig.PoolCheckInterval
	if interval <= 0 {
		interval = 30
	}
	poolConfig := PoolConfig{
		MinSize:       config.PoolConfig.PoolMinSize,
		MaxSize:       config.PoolConfig.PoolMaxSize,
		RefillSize:    config.PoolConfig.PoolRefillSize,
		CheckInterval: time.Duration(interval) * time.Second,
	}

	// 检查配置是否包含所需的模块
	selectedModule := config.SelectedModule

	// 初始化ASR池
	if asrType, ok := selectedModule["ASR"]; ok && asrType != "" {
		asrFactory := NewASRFactory(asrType, config, logger)
		if asrFactory == nil {
			return nil, fmt.Errorf("创建ASR工厂失败: 找不到配置 %s", asrType)
		}
		asrPool, err := NewResourcePool("asrPool", asrFactory, poolConfig, logger)
		if err != nil {
			return nil, fmt.Errorf("初始化ASR资源池失败: %v", err)
		}
		pm.asrPool = asrPool
		_, cnt := asrPool.GetStats()
		logger.Info("ASR资源池初始化成功，类型: %s, 数量：%d", asrType, cnt)
	}

	// 初始化LLM池
	if llmType, ok := selectedModule["LLM"]; ok && llmType != "" {
		llmFactory := NewLLMFactory(llmType, config, logger)
		if llmFactory == nil {
			return nil, fmt.Errorf("创建LLM工厂失败: 找不到配置 %s", llmType)
		}
		llmPool, err := NewResourcePool("llmPool", llmFactory, poolConfig, logger)
		if err != nil {
			return nil, fmt.Errorf("初始化LLM资源池失败: %v", err)
		}
		pm.llmPool = llmPool
		_, cnt := llmPool.GetStats()
		logger.Info("LLM资源池初始化成功，类型: %s, 数量：%d", llmType, cnt)
	}

	// 初始化TTS池
	if ttsType, ok := selectedModule["TTS"]; ok && ttsType != "" {
		ttsFactory := NewTTSFactory(ttsType, config, logger)
		if ttsFactory == nil {
			return nil, fmt.Errorf("创建TTS工厂失败: 找不到配置 %s", ttsType)
		}
		ttsPool, err := NewResourcePool("ttsPool", ttsFactory, poolConfig, logger)
		if err != nil {
			return nil, fmt.Errorf("初始化TTS资源池失败: %v", err)
		}
		pm.ttsPool = ttsPool
		_, cnt := ttsPool.GetStats()
		logger.Info("TTS资源池初始化成功，类型: %s, 数量：%d", ttsType, cnt)
	}

	// 初始化VLLLM池（可选）
	if vlllmType, ok := selectedModule["VLLLM"]; ok && vlllmType != "" {
		vlllmFactory := NewVLLLMFactory(vlllmType, config, logger)
		if vlllmFactory == nil {
			logger.Warn("创建VLLLM工厂失败: 找不到配置 %s", vlllmType)
		} else {
			vlllmPool, err := NewResourcePool("vllmPool", vlllmFactory, poolConfig, logger)
			if err != nil {
				logger.Warn("初始化VLLLM资源池失败（将继续使用普通LLM）: %v", err)
			} else {
				pm.vlllmPool = vlllmPool
			}
		}
		if pm.vlllmPool != nil {
			_, cnt := pm.vlllmPool.GetStats()
			logger.Info("VLLLM资源池初始化成功，类型: %s, 数量：%d", vlllmType, cnt)
		} else {
			logger.Warn("VLLLM资源池未初始化，将使用普通LLM")
		}
	}

	poolConfig = PoolConfig{
		MinSize:       config.McpPoolConfig.PoolMinSize,
		MaxSize:       config.McpPoolConfig.PoolMaxSize,
		RefillSize:    config.McpPoolConfig.PoolRefillSize,
		CheckInterval: time.Duration(config.McpPoolConfig.PoolCheckInterval) * time.Second,
	}

	// 初始化MCP池（总是初始化，因为MCP是核心功能）
	logger.Info("开始初始化MCP资源池，请等待...")
	mcpFactory := NewMCPFactory(config, logger)
	if mcpFactory != nil {
		mcpPool, err := NewResourcePool("mcpPool", mcpFactory, poolConfig, logger)
		if err != nil {
			return nil, fmt.Errorf("初始化MCP资源池失败: %v", err)
		}
		pm.mcpPool = mcpPool
		_, cnt := mcpPool.GetStats()
		logger.Info("MCP资源池初始化成功，数量：%d", cnt)
	} else {
		logger.Warn("创建MCP工厂失败，MCP功能将不可用")
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

	if pm.mcpPool != nil {
		mcpManager, err := pm.mcpPool.Get()
		if err == nil {
			// 直接转换，因为我们知道这是从 mcp 工厂创建的
			set.MCP = mcpManager.(*mcp.Manager)
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
	if pm.mcpPool != nil {
		pm.mcpPool.Close()
	}
}

// ReturnProviderSet 归还提供者集合到池中
func (pm *PoolManager) ReturnProviderSet(set *ProviderSet) error {
	if set == nil {
		return fmt.Errorf("提供者集合为空，无法归还")
	}

	var errs []error

	// 归还ASR提供者
	if set.ASR != nil && pm.asrPool != nil {
		// 重置资源状态
		if err := pm.asrPool.Reset(set.ASR); err != nil {
			pm.logger.Warn("重置ASR资源状态失败: %v", err)
		}
		// 归还到池中
		if err := pm.asrPool.Put(set.ASR); err != nil {
			errs = append(errs, fmt.Errorf("归还ASR提供者失败: %v", err))
			pm.logger.Error("归还ASR提供者失败: %v", err)
		} else {
			pm.logger.Debug("ASR提供者已成功归还到池中")
		}
	}

	// 归还LLM提供者
	if set.LLM != nil && pm.llmPool != nil {
		if err := pm.llmPool.Reset(set.LLM); err != nil {
			pm.logger.Warn("重置LLM资源状态失败: %v", err)
		}
		if err := pm.llmPool.Put(set.LLM); err != nil {
			errs = append(errs, fmt.Errorf("归还LLM提供者失败: %v", err))
			pm.logger.Error("归还LLM提供者失败: %v", err)
		} else {
			pm.logger.Debug("LLM提供者已成功归还到池中")
		}
	}

	// 归还TTS提供者
	if set.TTS != nil && pm.ttsPool != nil {
		if err := pm.ttsPool.Reset(set.TTS); err != nil {
			pm.logger.Warn("重置TTS资源状态失败: %v", err)
		}
		if err := pm.ttsPool.Put(set.TTS); err != nil {
			errs = append(errs, fmt.Errorf("归还TTS提供者失败: %v", err))
			pm.logger.Error("归还TTS提供者失败: %v", err)
		} else {
			pm.logger.Debug("TTS提供者已成功归还到池中")
		}
	}

	// 归还VLLLM提供者
	if set.VLLLM != nil && pm.vlllmPool != nil {
		if err := pm.vlllmPool.Reset(set.VLLLM); err != nil {
			pm.logger.Warn("重置VLLLM资源状态失败: %v", err)
		}
		if err := pm.vlllmPool.Put(set.VLLLM); err != nil {
			errs = append(errs, fmt.Errorf("归还VLLLM提供者失败: %v", err))
			pm.logger.Error("归还VLLLM提供者失败: %v", err)
		} else {
			pm.logger.Debug("VLLLM提供者已成功归还到池中")
		}
	}

	// 归还MCP提供者
	if set.MCP != nil && pm.mcpPool != nil {
		if err := pm.mcpPool.Reset(set.MCP); err != nil {
			pm.logger.Warn("重置MCP资源状态失败: %v", err)
		}
		if err := pm.mcpPool.Put(set.MCP); err != nil {
			errs = append(errs, fmt.Errorf("归还MCP提供者失败: %v", err))
			pm.logger.Error("归还MCP提供者失败: %v", err)
		} else {
			pm.logger.Debug("MCP提供者已成功归还到池中")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("归还过程中发生多个错误: %v", errs)
	}

	pm.logger.Debug("所有提供者已成功归还到池中")
	return nil
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

	if pm.mcpPool != nil {
		available, total := pm.mcpPool.GetStats()
		stats["mcp"] = map[string]int{"available": available, "total": total}
	}

	return stats
}

// performConnectivityCheck 执行连通性检查
func (pm *PoolManager) performConnectivityCheck(
	config *configs.Config,
	logger *utils.Logger,
) error {
	// 从配置创建连通性检查配置
	connConfig, err := ConfigFromYAML(&config.ConnectivityCheck)
	if err != nil {
		logger.Warn("解析连通性检查配置失败，使用默认配置: %v", err)
		connConfig = DefaultConnectivityConfig()
	}

	// 创建健康检查器
	healthChecker := NewHealthChecker(config, connConfig, logger)

	// 执行功能性连通性检查
	ctx, cancel := context.WithTimeout(context.Background(), connConfig.Timeout*3) // 给功能性检查更多时间
	defer cancel()

	err = healthChecker.CheckAllProviders(ctx, FunctionalCheck)

	// 打印检查报告
	healthChecker.PrintReport()

	return err
}

// GetDetailedStats 获取所有池的详细统计信息
func (pm *PoolManager) GetDetailedStats() map[string]map[string]int {
	stats := make(map[string]map[string]int)

	if pm.asrPool != nil {
		stats["asr"] = pm.asrPool.GetDetailedStats()
	}

	if pm.llmPool != nil {
		stats["llm"] = pm.llmPool.GetDetailedStats()
	}

	if pm.ttsPool != nil {
		stats["tts"] = pm.ttsPool.GetDetailedStats()
	}

	if pm.vlllmPool != nil {
		stats["vlllm"] = pm.vlllmPool.GetDetailedStats()
	}

	if pm.mcpPool != nil {
		stats["mcp"] = pm.mcpPool.GetDetailedStats()
	}

	return stats
}
