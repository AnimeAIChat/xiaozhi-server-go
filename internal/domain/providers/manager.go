package providers

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"
	"sync/atomic"

	domainmcp "xiaozhi-server-go/internal/domain/mcp"
	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/src/core/mcp"
	coreproviders "xiaozhi-server-go/src/core/providers"
	"xiaozhi-server-go/src/core/providers/asr"
	"xiaozhi-server-go/src/core/providers/llm"
	"xiaozhi-server-go/src/core/providers/tts"
	"xiaozhi-server-go/src/core/providers/vlllm"
	"xiaozhi-server-go/src/core/utils"
)

// Set groups together the provider instances required to serve a single
// conversational session. Consumers must release the set when finished so the
// underlying providers can be reused.
type Set struct {
	manager *Manager

	ASR   coreproviders.ASRProvider
	LLM   coreproviders.LLMProvider
	TTS   coreproviders.TTSProvider
	VLLLM *vlllm.Provider
	MCP   *domainmcp.Manager
}

// ReleaseWithContext returns the providers held by the set back to the manager.
func (s *Set) ReleaseWithContext(ctx context.Context) error {
	if s == nil || s.manager == nil {
		return nil
	}

	if ctx == nil {
		ctx = context.Background()
	}

	err := s.manager.release(ctx, s)
	s.manager = nil
	return err
}

// Release is a shorthand for ReleaseWithContext with a background context.
func (s *Set) Release() error {
	return s.ReleaseWithContext(context.Background())
}

func (s *Set) clear() {
	s.ASR = nil
	s.LLM = nil
	s.TTS = nil
	s.VLLLM = nil
	s.MCP = nil
}

// Manager coordinates provider creation and pooling for the application layer.
// It uses sync.Pool under the hood to avoid long-lived hot resources and
// relies on maps.Clone to ensure configuration maps are safely duplicated per
// provider instance.
type Manager struct {
	logger  *utils.Logger
	modules map[string]string

	asrPool   *providerPool[coreproviders.ASRProvider]
	llmPool   *providerPool[coreproviders.LLMProvider]
	ttsPool   *providerPool[coreproviders.TTSProvider]
	vlllmPool *providerPool[*vlllm.Provider]
	mcpPool   *providerPool[*domainmcp.Manager]

	closed atomic.Bool
}

// NewManager initialises the provider pools declared in the supplied config.
func NewManager(cfg *config.Config, logger *utils.Logger) (*Manager, error) {
	return NewManagerWithMCP(cfg, logger, nil)
}

// NewManagerWithMCP initialises the provider pools with an optional pre-initialised MCP manager.
func NewManagerWithMCP(cfg *config.Config, logger *utils.Logger, preInitMCPManager *mcp.Manager) (*Manager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("providers manager requires config")
	}

	if logger == nil {
		logger = utils.DefaultLogger
	}

	modules := map[string]string{}
	if cfg.Selected.ASR != "" {
		modules["ASR"] = cfg.Selected.ASR
	}
	if cfg.Selected.LLM != "" {
		modules["LLM"] = cfg.Selected.LLM
	}
	if cfg.Selected.TTS != "" {
		modules["TTS"] = cfg.Selected.TTS
	}
	if cfg.Selected.VLLLM != "" {
		modules["VLLLM"] = cfg.Selected.VLLLM
	}

	mgr := &Manager{
		logger:  logger,
		modules: modules,
	}

	var err error
	if mgr.asrPool, err = newASRPool(cfg, modules, logger); err != nil {
		return nil, err
	}
	if mgr.llmPool, err = newLLMPool(cfg, modules, logger); err != nil {
		return nil, err
	}
	if mgr.ttsPool, err = newTTSPool(cfg, modules, logger); err != nil {
		return nil, err
	}
	if mgr.vlllmPool, err = newVLLLMPool(cfg, modules, logger); err != nil {
		return nil, err
	}
	if mgr.mcpPool, err = newMCPPool(cfg, logger, preInitMCPManager); err != nil {
		return nil, err
	}

	return mgr, nil
}

// Acquire returns an initialised provider set for use by a websocket session.
func (m *Manager) Acquire(ctx context.Context) (*Set, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if m.closed.Load() {
		return nil, errors.New("providers manager closed")
	}

	set := &Set{manager: m}
	var err error

	if m.asrPool != nil {
		if set.ASR, err = m.asrPool.acquire(ctx); err != nil {
			m.releasePartial(ctx, set)
			return nil, err
		}
	}

	if m.llmPool != nil {
		if set.LLM, err = m.llmPool.acquire(ctx); err != nil {
			m.releasePartial(ctx, set)
			return nil, err
		}
	}

	if m.ttsPool != nil {
		if set.TTS, err = m.ttsPool.acquire(ctx); err != nil {
			m.releasePartial(ctx, set)
			return nil, err
		}
	}

	if m.vlllmPool != nil {
		if set.VLLLM, err = m.vlllmPool.acquire(ctx); err != nil {
			m.releasePartial(ctx, set)
			return nil, err
		}
	}

	if m.mcpPool != nil {
		if set.MCP, err = m.mcpPool.acquire(ctx); err != nil {
			m.releasePartial(ctx, set)
			return nil, err
		}
	}

	return set, nil
}

// AcquireMCP retrieves a standalone MCP manager from the underlying pool.
func (m *Manager) AcquireMCP(ctx context.Context) (*domainmcp.Manager, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if m.mcpPool == nil {
		return nil, fmt.Errorf("mcp pool not configured")
	}
	return m.mcpPool.acquire(ctx)
}

// ReleaseMCP returns an MCP manager to the pool.
func (m *Manager) ReleaseMCP(ctx context.Context, manager *domainmcp.Manager) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if manager == nil || m.mcpPool == nil {
		return nil
	}
	return m.mcpPool.release(ctx, manager)
}

// Close marks the manager as closed; subsequent Acquire calls will fail.
func (m *Manager) Close() {
	m.closed.Store(true)
}

// GetStats exposes a lightweight snapshot of pool utilisation for monitoring.
func (m *Manager) GetStats() map[string]map[string]int64 {
	stats := make(map[string]map[string]int64)

	if m.asrPool != nil {
		stats["asr"] = m.asrPool.stats()
	}
	if m.llmPool != nil {
		stats["llm"] = m.llmPool.stats()
	}
	if m.ttsPool != nil {
		stats["tts"] = m.ttsPool.stats()
	}
	if m.vlllmPool != nil {
		stats["vlllm"] = m.vlllmPool.stats()
	}
	if m.mcpPool != nil {
		stats["mcp"] = m.mcpPool.stats()
	}

	return stats
}

// Warmup pre-populates the provider pools to reduce latency on first requests.
func (m *Manager) Warmup(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// Warmup ASR pool
	if m.asrPool != nil {
		m.doWarmup(ctx, m.asrPool, "ASR")
	}

	// Warmup LLM pool
	if m.llmPool != nil {
		m.doWarmup(ctx, m.llmPool, "LLM")
	}

	// Warmup TTS pool
	if m.ttsPool != nil {
		m.doWarmup(ctx, m.ttsPool, "TTS")
	}

	// Warmup VLLLM pool
	if m.vlllmPool != nil {
		m.doWarmup(ctx, m.vlllmPool, "VLLLM")
	}

	// Warmup MCP pool
	if m.mcpPool != nil {
		m.doWarmup(ctx, m.mcpPool, "MCP")
	}

	return nil
}

// doWarmup performs the actual warmup for any provider pool
func (m *Manager) doWarmup(ctx context.Context, pool interface{}, poolType string) {
	// Use reflection to call the warmup logic
	// We need to use a helper function that can work with any pool type

	switch p := pool.(type) {
	case *providerPool[coreproviders.ASRProvider]:
		warmupConcretePool(ctx, p, poolType, m.logger)
	case *providerPool[coreproviders.LLMProvider]:
		warmupConcretePool(ctx, p, poolType, m.logger)
	case *providerPool[coreproviders.TTSProvider]:
		warmupConcretePool(ctx, p, poolType, m.logger)
	case *providerPool[*vlllm.Provider]:
		warmupConcretePool(ctx, p, poolType, m.logger)
	case *providerPool[*domainmcp.Manager]:
		warmupConcretePool(ctx, p, poolType, m.logger)
	default:
		m.logger.Warn("Unknown pool type for warmup: %T", pool)
	}
}

// warmupConcretePool is a generic function to warmup any concrete pool type
func warmupConcretePool[T any](ctx context.Context, pool *providerPool[T], poolType string, logger *utils.Logger) {
	warmSize := pool.warmSize

	for i := 0; i < warmSize; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Create a new resource
		resource, err := pool.create(ctx)
		if err != nil {
			if logger != nil {
				logger.Warn("Failed to create %s provider for warmup: %v", poolType, err)
			}
			continue
		}

		pool.created.Add(1)

		// Special handling for MCP Manager - try to pre-warm XiaoZhiMCPClient
		if poolType == "MCP" {
			if mcpManager, ok := any(resource).(*domainmcp.Manager); ok {
				warmupMCPManager(ctx, mcpManager, logger)
			}
		}

		// Try to put it in the warm pool
		select {
		case pool.warmPool <- resource:
			pool.warmed.Add(1)
			if logger != nil {
				logger.Debug("Warmed up %s provider %d/%d", poolType, i+1, warmSize)
			}
		default:
			// Warm pool is full, put in regular pool
			pool.pool.Put(resource)
			if logger != nil {
				logger.Debug("Warm pool full for %s, using regular pool", poolType)
			}
		}
	}

	if logger != nil {
		logger.Info("Successfully warmed up %s pool with %d instances", poolType, pool.warmed.Load())
	}
}

// warmupMCPManager attempts to pre-warm the XiaoZhiMCPClient within an MCP Manager
func warmupMCPManager(ctx context.Context, manager *domainmcp.Manager, logger *utils.Logger) {
	if manager == nil {
		return
	}

	// Try to pre-warm the XiaoZhiMCPClient by simulating a connection
	// This is a best-effort attempt - if it fails, the client will be warmed up during actual connection
	defer func() {
		if r := recover(); r != nil {
			if logger != nil {
				logger.Debug("MCP manager warmup failed (expected): %v", r)
			}
		}
	}()

	// Skip BindConnection during warmup to avoid configuration issues
	// The MCP manager will be properly initialized during actual connection
	if logger != nil {
		logger.Debug("Skipping MCP manager BindConnection during warmup")
	}
}

// mockMCPConnection implements the Conn interface for MCP warmup
type mockMCPConnection struct{}

func (m *mockMCPConnection) WriteMessage(messageType int, data []byte) error {
	// For warmup, we don't need to actually send messages
	// Just return success to avoid errors
	return nil
}

func (m *Manager) release(ctx context.Context, set *Set) error {
	var errs []error

	if set.ASR != nil && m.asrPool != nil {
		if err := m.asrPool.release(ctx, set.ASR); err != nil {
			errs = append(errs, err)
		}
		set.ASR = nil
	}

	if set.LLM != nil && m.llmPool != nil {
		if err := m.llmPool.release(ctx, set.LLM); err != nil {
			errs = append(errs, err)
		}
		set.LLM = nil
	}

	if set.TTS != nil && m.ttsPool != nil {
		if err := m.ttsPool.release(ctx, set.TTS); err != nil {
			errs = append(errs, err)
		}
		set.TTS = nil
	}

	if set.VLLLM != nil && m.vlllmPool != nil {
		if err := m.vlllmPool.release(ctx, set.VLLLM); err != nil {
			errs = append(errs, err)
		}
		set.VLLLM = nil
	}

	if set.MCP != nil && m.mcpPool != nil {
		if err := m.mcpPool.release(ctx, set.MCP); err != nil {
			errs = append(errs, err)
		}
		set.MCP = nil
	}

	set.clear()

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (m *Manager) releasePartial(ctx context.Context, set *Set) {
	if set == nil {
		return
	}

	if set.ASR != nil && m.asrPool != nil {
		m.asrPool.drop(ctx, set.ASR)
		set.ASR = nil
	}
	if set.LLM != nil && m.llmPool != nil {
		m.llmPool.drop(ctx, set.LLM)
		set.LLM = nil
	}
	if set.TTS != nil && m.ttsPool != nil {
		m.ttsPool.drop(ctx, set.TTS)
		set.TTS = nil
	}
	if set.VLLLM != nil && m.vlllmPool != nil {
		m.vlllmPool.drop(ctx, set.VLLLM)
		set.VLLLM = nil
	}
	if set.MCP != nil && m.mcpPool != nil {
		m.mcpPool.drop(ctx, set.MCP)
		set.MCP = nil
	}
	set.clear()
}

type providerPool[T any] struct {
	name    string
	logger  *utils.Logger
	create  func(context.Context) (T, error)
	reset   func(context.Context, T) error
	destroy func(T) error

	pool       sync.Pool      // 普通池，用于缓存已使用过的资源
	warmPool   chan T         // 预热池，存放预先创建的资源，带缓冲
	warmSize   int            // 预热池大小
	created    atomic.Int64
	inUse      atomic.Int64
	warmed     atomic.Int64 // 预热池中的资源数量
}

func newProviderPool[T any](
	name string,
	logger *utils.Logger,
	create func(context.Context) (T, error),
	reset func(context.Context, T) error,
	destroy func(T) error,
) *providerPool[T] {
	const defaultWarmSize = 1
	return &providerPool[T]{
		name:      name,
		logger:    logger,
		create:    create,
		reset:     reset,
		destroy:   destroy,
		warmPool:  make(chan T, defaultWarmSize),
		warmSize:  defaultWarmSize,
	}
}

func (p *providerPool[T]) acquire(ctx context.Context) (T, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// 1. 优先从预热池获取（非阻塞）
	select {
	case resource := <-p.warmPool:
		p.warmed.Add(-1)
		p.inUse.Add(1)
		if p.logger != nil {
			p.logger.Info("%s acquired from warm pool", p.name)
		}
		return resource, nil
	default:
		// 预热池为空，继续下一步
	}

	// 2. 从普通池获取（sync.Pool）
	if resource := p.pool.Get(); resource != nil {
		p.inUse.Add(1)
		if p.logger != nil {
			p.logger.Info("%s acquired from regular pool", p.name)
		}
		return resource.(T), nil
	}

	// 3. 动态创建新资源
	res, err := p.create(ctx)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("%s create: %w", p.name, err)
	}

	p.created.Add(1)
	p.inUse.Add(1)
	if p.logger != nil {
		p.logger.Info("%s created new instance", p.name)
	}
	return res, nil
}

func (p *providerPool[T]) release(ctx context.Context, resource T) error {
	if ctx == nil {
		ctx = context.Background()
	}

	defer p.inUse.Add(-1)

	if p.reset != nil {
		if err := p.reset(ctx, resource); err != nil {
			if p.logger != nil {
				p.logger.Warn("%s reset failed: %v", p.name, err)
			}
			if p.destroy != nil {
				if derr := p.destroy(resource); derr != nil && p.logger != nil {
					p.logger.Error("%s destroy failed after reset error: %v", p.name, derr)
				}
			}
			return fmt.Errorf("%s reset failed: %w", p.name, err)
		}
	}

	// 优先放回预热池，如果预热池未满
	select {
	case p.warmPool <- resource:
		p.warmed.Add(1)
		if p.logger != nil {
			p.logger.Info("%s returned to warm pool", p.name)
		}
		return nil
	default:
		// 预热池已满，放回普通池
		p.pool.Put(resource)
		if p.logger != nil {
			p.logger.Info("%s returned to regular pool", p.name)
		}
		return nil
	}
}

func (p *providerPool[T]) drop(ctx context.Context, resource T) {
	p.inUse.Add(-1)
	if p.destroy != nil {
		if err := p.destroy(resource); err != nil && p.logger != nil {
			p.logger.Error("%s destroy failed: %v", p.name, err)
		}
	}
}

func (p *providerPool[T]) stats() map[string]int64 {
	total := p.created.Load()
	inUse := p.inUse.Load()
	warmed := p.warmed.Load()
	available := total - inUse
	if available < 0 {
		available = 0
	}
	return map[string]int64{
		"total":     total,
		"in_use":    inUse,
		"available": available,
		"warmed":    warmed,
	}
}

func newASRPool(
	cfg *config.Config,
	modules map[string]string,
	logger *utils.Logger,
) (*providerPool[coreproviders.ASRProvider], error) {
	name, ok := modules["ASR"]
	if !ok || name == "" {
		return nil, nil
	}

	asrCfg, ok := cfg.ASR[name]
	if !ok {
		return nil, fmt.Errorf("selected ASR provider %q is not configured", name)
	}

	asrCfgMap, ok := asrCfg.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("ASR provider %q configuration is not a map", name)
	}

	providerType, _ := asrCfgMap["type"].(string)
	if providerType == "" {
		providerType = name
	}

	create := func(ctx context.Context) (coreproviders.ASRProvider, error) {
		data := maps.Clone(asrCfgMap)
		provider, err := asr.Create(
			providerType,
			&asr.Config{
				Name: name,
				Type: providerType,
				Data: data,
			},
			cfg.Audio.DeleteAudio,
			logger,
		)
		if err != nil {
			return nil, err
		}
		coreProvider, ok := provider.(coreproviders.ASRProvider)
		if !ok {
			return nil, fmt.Errorf("asr provider %s does not implement coreproviders.ASRProvider", name)
		}
		return coreProvider, nil
	}

	reset := func(ct context.Context, provider coreproviders.ASRProvider) error {
		if resetter, ok := any(provider).(interface{ Reset() error }); ok {
			return resetter.Reset()
		}
		return nil
	}

	destroy := func(provider coreproviders.ASRProvider) error {
		if provider == nil {
			return nil
		}
		return provider.Cleanup()
	}

	return newProviderPool("asr:"+name, logger, create, reset, destroy), nil
}

func newLLMPool(
	cfg *config.Config,
	modules map[string]string,
	logger *utils.Logger,
) (*providerPool[coreproviders.LLMProvider], error) {
	name, ok := modules["LLM"]
	if !ok || name == "" {
		return nil, nil
	}

	llmCfg, ok := cfg.LLM[name]
	if !ok {
		return nil, fmt.Errorf("selected LLM provider %q is not configured", name)
	}

	create := func(ctx context.Context) (coreproviders.LLMProvider, error) {
		extra := maps.Clone(llmCfg.Extra)
		provider, err := llm.Create(
			llmCfg.Type,
			&llm.Config{
				Name:        name,
				Type:        llmCfg.Type,
				ModelName:   llmCfg.ModelName,
				BaseURL:     llmCfg.BaseURL,
				APIKey:      llmCfg.APIKey,
				Temperature: llmCfg.Temperature,
				MaxTokens:   llmCfg.MaxTokens,
				TopP:        llmCfg.TopP,
				Extra:       extra,
			},
		)
		if err != nil {
			return nil, err
		}

		// 初始化提供者
		if initializer, ok := any(provider).(interface{ Initialize() error }); ok {
			if err := initializer.Initialize(); err != nil {
				return nil, fmt.Errorf("failed to initialize LLM provider %s: %w", name, err)
			}
		}

		return provider, nil
	}

	reset := func(ct context.Context, provider coreproviders.LLMProvider) error {
		if resetter, ok := any(provider).(interface{ Reset() error }); ok {
			return resetter.Reset()
		}
		return nil
	}

	destroy := func(provider coreproviders.LLMProvider) error {
		if provider == nil {
			return nil
		}
		if cleaner, ok := any(provider).(interface{ Cleanup() error }); ok {
			return cleaner.Cleanup()
		}
		return nil
	}

	return newProviderPool("llm:"+name, logger, create, reset, destroy), nil
}

func newTTSPool(
	cfg *config.Config,
	modules map[string]string,
	logger *utils.Logger,
) (*providerPool[coreproviders.TTSProvider], error) {
	name, ok := modules["TTS"]
	if !ok || name == "" {
		return nil, nil
	}

	ttsCfg, ok := cfg.TTS[name]
	if !ok {
		return nil, fmt.Errorf("selected TTS provider %q is not configured", name)
	}

	create := func(ctx context.Context) (coreproviders.TTSProvider, error) {
		provider, err := tts.Create(
			ttsCfg.Type,
			&tts.Config{
				Name:            name,
				Type:            ttsCfg.Type,
				OutputDir:       ttsCfg.OutputDir,
				Voice:           ttsCfg.Voice,
				Format:          ttsCfg.Format,
				AppID:           ttsCfg.AppID,
				Token:           ttsCfg.Token,
				Cluster:         ttsCfg.Cluster,
				SupportedVoices: ttsCfg.SupportedVoices,
			},
			cfg.Audio.DeleteAudio,
		)
		if err != nil {
			return nil, err
		}
		return provider, nil
	}

	reset := func(ct context.Context, provider coreproviders.TTSProvider) error {
		if resetter, ok := any(provider).(interface{ Reset() error }); ok {
			return resetter.Reset()
		}
		return nil
	}

	destroy := func(provider coreproviders.TTSProvider) error {
		if provider == nil {
			return nil
		}
		return provider.Cleanup()
	}

	return newProviderPool("tts:"+name, logger, create, reset, destroy), nil
}

func newVLLLMPool(
	cfg *config.Config,
	modules map[string]string,
	logger *utils.Logger,
) (*providerPool[*vlllm.Provider], error) {
	name, ok := modules["VLLLM"]
	if !ok || name == "" {
		return nil, nil
	}

	vCfg, ok := cfg.VLLLM[name]
	if !ok {
		return nil, fmt.Errorf("selected VLLLM provider %q is not configured", name)
	}

	create := func(ctx context.Context) (*vlllm.Provider, error) {
		cfgCopy := vCfg
		cfgCopy.Extra = maps.Clone(vCfg.Extra)
		return vlllm.Create(cfgCopy.Type, &cfgCopy, logger)
	}

	reset := func(ct context.Context, provider *vlllm.Provider) error {
		if provider == nil {
			return nil
		}
		return provider.Initialize()
	}

	destroy := func(provider *vlllm.Provider) error {
		if provider == nil {
			return nil
		}
		return provider.Cleanup()
	}

	return newProviderPool("vlllm:"+name, logger, create, reset, destroy), nil
}

func newMCPPool(
	cfg *config.Config,
	logger *utils.Logger,
	preInitMCPManager *mcp.Manager,
) (*providerPool[*domainmcp.Manager], error) {
	create := func(ctx context.Context) (*domainmcp.Manager, error) {
		if preInitMCPManager != nil {
			return domainmcp.NewFromManager(preInitMCPManager, logger)
		}
		return domainmcp.NewFromConfig(cfg, logger)
	}

	reset := func(ct context.Context, manager *domainmcp.Manager) error {
		if manager == nil {
			return nil
		}
		return manager.Reset()
	}

	destroy := func(manager *domainmcp.Manager) error {
		if manager == nil {
			return nil
		}
		return manager.Cleanup()
	}

	return newProviderPool("mcp", logger, create, reset, destroy), nil
}
