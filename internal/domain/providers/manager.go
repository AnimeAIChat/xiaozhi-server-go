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

	minSize := int64(2) // Default minimum pool size

	// Warmup ASR pool
	if m.asrPool != nil {
		for i := int64(0); i < minSize; i++ {
			provider, err := m.asrPool.acquire(ctx)
			if err != nil {
				m.logger.Warn("Failed to warmup ASR provider: %v", err)
				continue
			}
			if err := m.asrPool.release(ctx, provider); err != nil {
				m.logger.Warn("Failed to release warmed up ASR provider: %v", err)
			}
		}
	}

	// Warmup LLM pool
	if m.llmPool != nil {
		for i := int64(0); i < minSize; i++ {
			provider, err := m.llmPool.acquire(ctx)
			if err != nil {
				m.logger.Warn("Failed to warmup LLM provider: %v", err)
				continue
			}
			if err := m.llmPool.release(ctx, provider); err != nil {
				m.logger.Warn("Failed to release warmed up LLM provider: %v", err)
			}
		}
	}

	// Warmup TTS pool
	if m.ttsPool != nil {
		for i := int64(0); i < minSize; i++ {
			provider, err := m.ttsPool.acquire(ctx)
			if err != nil {
				m.logger.Warn("Failed to warmup TTS provider: %v", err)
				continue
			}
			if err := m.ttsPool.release(ctx, provider); err != nil {
				m.logger.Warn("Failed to release warmed up TTS provider: %v", err)
			}
		}
	}

	// Warmup VLLLM pool
	if m.vlllmPool != nil {
		for i := int64(0); i < minSize; i++ {
			provider, err := m.vlllmPool.acquire(ctx)
			if err != nil {
				m.logger.Warn("Failed to warmup VLLLM provider: %v", err)
				continue
			}
			if err := m.vlllmPool.release(ctx, provider); err != nil {
				m.logger.Warn("Failed to release warmed up VLLLM provider: %v", err)
			}
		}
	}

	// Warmup MCP pool
	if m.mcpPool != nil {
		for i := int64(0); i < minSize; i++ {
			manager, err := m.mcpPool.acquire(ctx)
			if err != nil {
				m.logger.Warn("Failed to warmup MCP manager: %v", err)
				continue
			}
			if err := m.mcpPool.release(ctx, manager); err != nil {
				m.logger.Warn("Failed to release warmed up MCP manager: %v", err)
			}
		}
	}

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

	pool    sync.Pool
	created atomic.Int64
	inUse   atomic.Int64
}

func newProviderPool[T any](
	name string,
	logger *utils.Logger,
	create func(context.Context) (T, error),
	reset func(context.Context, T) error,
	destroy func(T) error,
) *providerPool[T] {
	return &providerPool[T]{
		name:    name,
		logger:  logger,
		create:  create,
		reset:   reset,
		destroy: destroy,
	}
}

func (p *providerPool[T]) acquire(ctx context.Context) (T, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if resource := p.pool.Get(); resource != nil {
		p.inUse.Add(1)
		return resource.(T), nil
	}

	res, err := p.create(ctx)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("%s create: %w", p.name, err)
	}

	p.created.Add(1)
	p.inUse.Add(1)
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

	p.pool.Put(resource)
	return nil
}

func (p *providerPool[T]) drop(ctx context.Context, resource T) {
	if ctx == nil {
		ctx = context.Background()
	}

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
	available := total - inUse
	if available < 0 {
		available = 0
	}
	return map[string]int64{
		"total":     total,
		"in_use":    inUse,
		"available": available,
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
