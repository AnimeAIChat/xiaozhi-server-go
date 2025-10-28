package pool

import (
	"context"
	"fmt"

	domainmcp "xiaozhi-server-go/internal/domain/mcp"
	domainproviders "xiaozhi-server-go/internal/domain/providers"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core/utils"
)

// ProviderSet is kept for backwards compatibility with legacy callers under src/core.
type ProviderSet = domainproviders.Set

// PoolManager bridges historical pool APIs to the refactored domain provider manager.
type PoolManager struct {
	manager *domainproviders.Manager
	logger  *utils.Logger
}

// NewPoolManager builds a shim over the new provider manager while keeping the
// existing constructor signature intact.
func NewPoolManager(cfg *configs.Config, logger *utils.Logger) (*PoolManager, error) {
	manager, err := domainproviders.NewManager(cfg, logger)
	if err != nil {
		return nil, err
	}
	return &PoolManager{
		manager: manager,
		logger:  logger,
	}, nil
}

// GetProviderSet fetches a provider set for the caller. The returned set must
// be released via ReturnProviderSet once finished.
func (pm *PoolManager) GetProviderSet() (*ProviderSet, error) {
	if pm == nil || pm.manager == nil {
		return nil, fmt.Errorf("pool manager not initialised")
	}
	return pm.manager.Acquire(context.Background())
}

// ReturnProviderSet releases an acquired set back to the underlying pools.
func (pm *PoolManager) ReturnProviderSet(set *ProviderSet) error {
	if set == nil {
		return nil
	}
	return set.Release()
}

// GetMcpManager provides compatibility for legacy transport code that expects
// to borrow a standalone MCP manager.
func (pm *PoolManager) GetMcpManager() (*domainmcp.Manager, error) {
	if pm == nil || pm.manager == nil {
		return nil, fmt.Errorf("pool manager not initialised")
	}
	return pm.manager.AcquireMCP(context.Background())
}

// ReturnMcpManager returns a borrowed MCP manager.
func (pm *PoolManager) ReturnMcpManager(m *domainmcp.Manager) error {
	if pm == nil || pm.manager == nil || m == nil {
		return nil
	}
	return pm.manager.ReleaseMCP(context.Background(), m)
}

// Close signals the manager to stop servicing new requests.
func (pm *PoolManager) Close() {
	if pm == nil || pm.manager == nil {
		return
	}
	pm.manager.Close()
}

// GetStats mirrors the legacy API, providing integer statistics for telemetry.
func (pm *PoolManager) GetStats() map[string]map[string]int {
	if pm == nil || pm.manager == nil {
		return map[string]map[string]int{}
	}

	stats64 := pm.manager.GetStats()
	stats := make(map[string]map[string]int, len(stats64))
	for name, values := range stats64 {
		stats[name] = map[string]int{
			"available": int(values["available"]),
			"in_use":    int(values["in_use"]),
			"total":     int(values["total"]),
		}
	}
	return stats
}

// GetDetailedStats currently aliases GetStats to maintain backwards compatibility.
func (pm *PoolManager) GetDetailedStats() map[string]map[string]int {
	return pm.GetStats()
}
