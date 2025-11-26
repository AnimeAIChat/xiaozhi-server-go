package manager

import (
	"context"
	"fmt"
	"sync"
	"time"

	contractProviders "xiaozhi-server-go/internal/contracts/providers"
	"xiaozhi-server-go/internal/domain/asr/infrastructure/adapters"
	"xiaozhi-server-go/internal/domain/llm/infrastructure/adapters/openai"
	"xiaozhi-server-go/internal/domain/tts/infrastructure/adapters/edge"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/internal/utils"
)

// UnifiedProviderManager 统一提供者管理器
// 实现新的Provider系统，替代旧的资源池管理器
type UnifiedProviderManager struct {
	sessionID      string
	config         *config.Config
	logger         *utils.Logger
	isInitialized  bool

	// 提供者注册表
	asrRegistry *asr.ASRRegistry
	llmFactories map[string]contractProviders.LLMProviderFactory
	ttsFactories map[string]contractProviders.TTSProviderFactory

	// 活跃的提供者实例
	activeASRProviders map[string]contractProviders.ASRProvider
	activeLLMProviders map[string]contractProviders.LLMProvider
	activeTTSProviders map[string]contractProviders.TTSProvider

	// 性能监控
	stats *ProviderStats
	mutex sync.RWMutex

	// 生命周期管理
	ctx    context.Context
	cancel context.CancelFunc
}

// ProviderStats 提供者统计信息
type ProviderStats struct {
	TotalRequests     int64 `json:"total_requests"`
	SuccessfulRequests int64 `json:"successful_requests"`
	FailedRequests    int64 `json:"failed_requests"`
	CacheHits         int64 `json:"cache_hits"`
	AverageLatency    time.Duration `json:"average_latency"`
	mutex             sync.RWMutex
	requestLatencies  []time.Duration
}

// Config 提供者管理器配置
type Config struct {
	EnableCaching    bool          `json:"enable_caching" yaml:"enable_caching"`
	CacheSize        int           `json:"cache_size" yaml:"cache_size"`
	CacheTTL         time.Duration `json:"cache_ttl" yaml:"cache_ttl"`
	EnableMetrics    bool          `json:"enable_metrics" yaml:"enable_metrics"`
	HealthCheckInterval time.Duration `json:"health_check_interval" yaml:"health_check_interval"`
}

// NewUnifiedProviderManager 创建统一提供者管理器
func NewUnifiedProviderManager(cfg *config.Config, logger *utils.Logger) *UnifiedProviderManager {
	if logger == nil {
		logger = utils.DefaultLogger
	}

	ctx, cancel := context.WithCancel(context.Background())

	manager := &UnifiedProviderManager{
		sessionID:         fmt.Sprintf("provider-manager-%d", time.Now().UnixNano()),
		config:            cfg,
		logger:            logger,
		isInitialized:     false,
		asrRegistry:       asr.NewASRRegistry(),
		llmFactories:      make(map[string]contractProviders.LLMProviderFactory),
		ttsFactories:      make(map[string]contractProviders.TTSProviderFactory),
		activeASRProviders: make(map[string]contractProviders.ASRProvider),
		activeLLMProviders: make(map[string]contractProviders.LLMProvider),
		activeTTSProviders: make(map[string]contractProviders.TTSProvider),
		stats:             &ProviderStats{},
		ctx:               ctx,
		cancel:            cancel,
	}

	// 注册内置提供者
	manager.registerBuiltinProviders()

	return manager
}

// Initialize 初始化管理器
func (m *UnifiedProviderManager) Initialize() error {
	m.logger.InfoTag("ProviderManager", "初始化统一提供者管理器，SessionID: %s", m.sessionID)

	// 预热常用的提供者
	if err := m.warmupProviders(); err != nil {
		m.logger.WarnTag("ProviderManager", "提供者预热部分失败: %v", err)
	}

	// 启动健康检查
	go m.startHealthCheckRoutine()

	m.isInitialized = true
	m.logger.InfoTag("ProviderManager", "统一提供者管理器初始化完成，SessionID: %s", m.sessionID)
	return nil
}

// Cleanup 清理管理器资源
func (m *UnifiedProviderManager) Cleanup() error {
	m.logger.InfoTag("ProviderManager", "清理统一提供者管理器，SessionID: %s", m.sessionID)

	// 取消上下文
	m.cancel()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 清理所有活跃的提供者
	m.cleanupProviders(m.activeASRProviders)
	m.cleanupProviders(m.activeLLMProviders)
	m.cleanupProviders(m.activeTTSProviders)

	// 清空提供者映射
	m.activeASRProviders = make(map[string]contractProviders.ASRProvider)
	m.activeLLMProviders = make(map[string]contractProviders.LLMProvider)
	m.activeTTSProviders = make(map[string]contractProviders.TTSProvider)

	m.isInitialized = false
	return nil
}

// CreateASRProvider 创建ASR提供者
func (m *UnifiedProviderManager) CreateASRProvider(providerType string, config interface{}, options map[string]interface{}) (contractProviders.ASRProvider, error) {
	if !m.isInitialized {
		return nil, fmt.Errorf("provider manager not initialized")
	}

	m.logger.DebugTag("ProviderManager", "创建ASR提供者: %s", providerType)

	provider, err := m.asrRegistry.CreateProvider(providerType, config, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create ASR provider: %w", err)
	}

	// 记录统计信息
	m.stats.recordRequest()

	return provider, nil
}

// CreateLLMProvider 创建LLM提供者
func (m *UnifiedProviderManager) CreateLLMProvider(providerType string, config interface{}, options map[string]interface{}) (contractProviders.LLMProvider, error) {
	if !m.isInitialized {
		return nil, fmt.Errorf("provider manager not initialized")
	}

	m.logger.DebugTag("ProviderManager", "创建LLM提供者: %s", providerType)

	factory, exists := m.llmFactories[providerType]
	if !exists {
		return nil, fmt.Errorf("LLM provider factory '%s' not found", providerType)
	}

	provider, err := factory.CreateProvider(config, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM provider: %w", err)
	}

	// 记录统计信息
	m.stats.recordRequest()

	return provider, nil
}

// CreateTTSProvider 创建TTS提供者
func (m *UnifiedProviderManager) CreateTTSProvider(providerType string, config interface{}, options map[string]interface{}) (contractProviders.TTSProvider, error) {
	if !m.isInitialized {
		return nil, fmt.Errorf("provider manager not initialized")
	}

	m.logger.DebugTag("ProviderManager", "创建TTS提供者: %s", providerType)

	factory, exists := m.ttsFactories[providerType]
	if !exists {
		return nil, fmt.Errorf("TTS provider factory '%s' not found", providerType)
	}

	provider, err := factory.CreateProvider(config, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create TTS provider: %w", err)
	}

	// 记录统计信息
	m.stats.recordRequest()

	return provider, nil
}

// GetAvailableProviders 获取可用的提供者列表
func (m *UnifiedProviderManager) GetAvailableProviders() map[string][]string {
	return map[string][]string{
		"asr": m.asrRegistry.ListFactories(),
		"llm": m.getLLMProviderNames(),
		"tts": m.getTTSProviderNames(),
	}
}

// HealthCheck 执行健康检查
func (m *UnifiedProviderManager) HealthCheck(ctx context.Context) map[string]error {
	results := make(map[string]error)

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// 检查ASR提供者
	for name, provider := range m.activeASRProviders {
		if err := provider.HealthCheck(ctx); err != nil {
			results[fmt.Sprintf("asr:%s", name)] = err
		}
	}

	// 检查LLM提供者
	for name, provider := range m.activeLLMProviders {
		if err := provider.HealthCheck(ctx); err != nil {
			results[fmt.Sprintf("llm:%s", name)] = err
		}
	}

	// 检查TTS提供者
	for name, provider := range m.activeTTSProviders {
		if err := provider.HealthCheck(ctx); err != nil {
			results[fmt.Sprintf("tts:%s", name)] = err
		}
	}

	return results
}

// GetStats 获取统计信息
func (m *UnifiedProviderManager) GetStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return map[string]interface{}{
		"total_requests":       m.stats.TotalRequests,
		"successful_requests":  m.stats.SuccessfulRequests,
		"failed_requests":      m.stats.FailedRequests,
		"cache_hits":           m.stats.CacheHits,
		"average_latency":      m.stats.AverageLatency.String(),
		"active_asr_providers": len(m.activeASRProviders),
		"active_llm_providers": len(m.activeLLMProviders),
		"active_tts_providers": len(m.activeTTSProviders),
	}
}

// Warmup 预热提供者
func (m *UnifiedProviderManager) Warmup(ctx context.Context) error {
	return m.warmupProviders()
}

// registerBuiltinProviders 注册内置提供者
func (m *UnifiedProviderManager) registerBuiltinProviders() {
	// 注册LLM提供者
	openaiFactory := openai.NewOpenAILLMFactory()
	m.llmFactories[openaiFactory.GetProviderName()] = openaiFactory

	// 注册TTS提供者
	edgeFactory := edge.NewEdgeTTSFactory()
	m.ttsFactories[edgeFactory.GetProviderName()] = edgeFactory

	m.logger.InfoTag("ProviderManager", "已注册内置提供者")
}

// warmupProviders 预热提供者
func (m *UnifiedProviderManager) warmupProviders() error {
	options := map[string]interface{}{
		"logger": m.logger,
	}

	// 预热LLM提供者（如果配置了）
	if m.config.LLM.OpenAI.APIKey != "" {
		llmProvider, err := m.CreateLLMProvider("openai", m.config, options)
		if err != nil {
			m.logger.WarnTag("ProviderManager", "预热OpenAI LLM提供者失败: %v", err)
		} else {
			m.activeLLMProviders["openai"] = llmProvider
			m.logger.InfoTag("ProviderManager", "OpenAI LLM提供者预热完成")
		}
	}

	// 预热TTS提供者
	ttsProvider, err := m.CreateTTSProvider("edge", m.config, options)
	if err != nil {
		m.logger.WarnTag("ProviderManager", "预热Edge TTS提供者失败: %v", err)
	} else {
		m.activeTTSProviders["edge"] = ttsProvider
		m.logger.InfoTag("ProviderManager", "Edge TTS提供者预热完成")
	}

	return nil
}

// startHealthCheckRoutine 启动健康检查例行程序
func (m *UnifiedProviderManager) startHealthCheckRoutine() {
	ticker := time.NewTicker(5 * time.Minute) // 每5分钟检查一次
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
			results := m.HealthCheck(ctx)
			cancel()

			// 记录健康检查结果
			healthyCount := 0
			totalCount := len(results)
			for provider, err := range results {
				if err != nil {
					m.logger.WarnTag("ProviderManager", "健康检查失败 - %s: %v", provider, err)
				} else {
					healthyCount++
				}
			}

			if totalCount > 0 {
				m.logger.DebugTag("ProviderManager", "健康检查完成: %d/%d 提供者健康", healthyCount, totalCount)
			}
		}
	}
}

// cleanupProviders 清理提供者
func (m *UnifiedProviderManager) cleanupProviders(providers map[string]interface{}) {
	for name, provider := range providers {
		if closer, ok := provider.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				m.logger.WarnTag("ProviderManager", "关闭提供者 %s 失败: %v", name, err)
			}
		}
	}
}

// getLLMProviderNames 获取LLM提供者名称列表
func (m *UnifiedProviderManager) getLLMProviderNames() []string {
	names := make([]string, 0, len(m.llmFactories))
	for name := range m.llmFactories {
		names = append(names, name)
	}
	return names
}

// getTTSProviderNames 获取TTS提供者名称列表
func (m *UnifiedProviderManager) getTTSProviderNames() []string {
	names := make([]string, 0, len(m.ttsFactories))
	for name := range m.ttsFactories {
		names = append(names, name)
	}
	return names
}

// ProviderStats 方法实现

// recordRequest 记录请求
func (ps *ProviderStats) recordRequest() {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	ps.TotalRequests++
}

// recordSuccess 记录成功
func (ps *ProviderStats) recordSuccess(latency time.Duration) {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	ps.SuccessfulRequests++
	ps.addLatency(latency)
}

// recordFailure 记录失败
func (ps *ProviderStats) recordFailure() {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	ps.FailedRequests++
}

// recordCacheHit 记录缓存命中
func (ps *ProviderStats) recordCacheHit() {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	ps.CacheHits++
}

// addLatency 添加延迟记录
func (ps *ProviderStats) addLatency(latency time.Duration) {
	ps.requestLatencies = append(ps.requestLatencies, latency)

	// 保持最近1000个记录
	if len(ps.requestLatencies) > 1000 {
		ps.requestLatencies = ps.requestLatencies[1:]
	}

	// 计算平均延迟
	if len(ps.requestLatencies) > 0 {
		var total time.Duration
		for _, l := range ps.requestLatencies {
			total += l
		}
		ps.AverageLatency = total / time.Duration(len(ps.requestLatencies))
	}
}