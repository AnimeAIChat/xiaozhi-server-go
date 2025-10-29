package pool

import (
	"fmt"
	"time"
	"xiaozhi-server-go/internal/util"
	"xiaozhi-server-go/src/core/utils"
)

/*
* 资源池资源类
* 提供完整的资源生命周期管理、健康检查、超时控制等功能
* 兼容原有接口，支持动态扩展和收缩
 */

// ResourceFactory 资源工厂接口 - 适配器模式
type ResourceFactory interface {
	Create() (interface{}, error)
	Destroy(resource interface{}) error
}

// LegacyResourceAdapter 适配器：将旧接口适配到新接口
type legacyResourceAdapter struct {
	factory ResourceFactory
}

// Create 实现新ResourceFactory接口
func (a *legacyResourceAdapter) Create() (util.Resource, error) {
	resource, err := a.factory.Create()
	if err != nil {
		return nil, err
	}
	return &legacyResourceWrapper{resource: resource, factory: a.factory}, nil
}

// Validate 验证资源有效性
func (a *legacyResourceAdapter) Validate(resource util.Resource) bool {
	if wrapper, ok := resource.(*legacyResourceWrapper); ok {
		// 如果资源实现了验证接口，使用它
		if validator, ok := wrapper.resource.(interface{ IsValid() bool }); ok {
			return validator.IsValid()
		}
		// 默认认为有效
		return true
	}
	return false
}

// Reset 重置资源状态
func (a *legacyResourceAdapter) Reset(resource util.Resource) error {
	if wrapper, ok := resource.(*legacyResourceWrapper); ok {
		// 如果资源实现了重置接口，使用它
		if resetter, ok := wrapper.resource.(interface{ Reset() error }); ok {
			return resetter.Reset()
		}
		// 默认无操作
		return nil
	}
	return nil
}

// legacyResourceWrapper 包装器：将interface{}包装成Resource接口
type legacyResourceWrapper struct {
	resource interface{}
	factory  ResourceFactory
}

// Close 关闭资源
func (w *legacyResourceWrapper) Close() error {
	return w.factory.Destroy(w.resource)
}

// IsValid 检查资源是否有效
func (w *legacyResourceWrapper) IsValid() bool {
	// 如果资源实现了验证接口，使用它
	if validator, ok := w.resource.(interface{ IsValid() bool }); ok {
		return validator.IsValid()
	}
	// 默认认为有效
	return true
}

// ResourcePool 通用资源池 - 基于新实现的适配器
type ResourcePool struct {
	poolName string
	pool     *util.ResourcePool
	logger   *utils.Logger
}

// PoolConfig 资源池配置 - 适配旧配置格式
type PoolConfig struct {
	MinSize       int           // 最小资源数量
	MaxSize       int           // 最大资源数量
	RefillSize    int           // 补充阈值（兼容旧接口，不再使用）
	CheckInterval time.Duration // 检查间隔（兼容旧接口，不再使用）
}

// NewResourcePool 创建资源池
func NewResourcePool(
	poolName string,
	factory ResourceFactory,
	config PoolConfig,
	logger *utils.Logger,
) (*ResourcePool, error) {
	// 创建适配器
	adapter := &legacyResourceAdapter{factory: factory}

	// 转换配置
	poolConfig := &util.PoolConfig{
		MaxSize:          config.MaxSize,
		MinSize:          config.MinSize,
		MaxIdle:          config.MaxSize, // 允许所有资源空闲
		AcquireTimeout:   30 * time.Second,
		IdleTimeout:      5 * time.Minute,
		ValidateOnBorrow: true,
		ValidateOnReturn: false,
	}

	// 创建底层资源池
	underlyingPool, err := util.NewResourcePool(poolConfig, adapter)
	if err != nil {
		return nil, fmt.Errorf("创建资源池失败: %w", err)
	}

	return &ResourcePool{
		poolName: poolName,
		pool:     underlyingPool,
		logger:   logger,
	}, nil
}

// Get 获取资源
func (p *ResourcePool) Get() (interface{}, error) {
	resource, err := p.pool.Acquire()
	if err != nil {
		return nil, err
	}

	// 解包获取原始资源
	if wrapper, ok := resource.(*legacyResourceWrapper); ok {
		return wrapper.resource, nil
	}

	return resource, nil
}

// Put 将资源归还到池中
func (p *ResourcePool) Put(resource interface{}) error {
	if resource == nil {
		return fmt.Errorf("%s 不能将nil资源归还到池中", p.poolName)
	}

	// 包装资源
	wrapper := &legacyResourceWrapper{
		resource: resource,
		factory:  nil, // 在Release时不需要factory
	}

	return p.pool.Release(wrapper)
}

// Reset 重置资源状态（在归还前调用）
func (p *ResourcePool) Reset(resource interface{}) error {
	if resource == nil {
		return nil
	}

	// 如果资源实现了重置接口，直接调用
	if resetter, ok := resource.(interface{ Reset() error }); ok {
		return resetter.Reset()
	}

	// 否则通过池的Reset方法
	// 注意：这里我们简化处理，实际应该在创建时保存factory引用
	return nil
}

// GetStats 获取池状态
func (p *ResourcePool) GetStats() (available, total int) {
	stats := p.pool.Stats()
	availableRes := stats["available_resources"].(int)
	totalRes := stats["total_resources"].(int)

	return availableRes, totalRes
}

// GetDetailedStats 获取详细的池状态
func (p *ResourcePool) GetDetailedStats() map[string]int {
	stats := p.pool.Stats()

	return map[string]int{
		"available": stats["available_resources"].(int),
		"total":     stats["total_resources"].(int),
		"max":       stats["max_size"].(int),
		"min":       stats["min_size"].(int),
		"in_use":    stats["in_use_resources"].(int),
	}
}

// Close 关闭资源池
func (p *ResourcePool) Close() error {
	return p.pool.Close()
}

// GetPoolStats 获取完整的池统计信息（新方法）
func (p *ResourcePool) GetPoolStats() map[string]interface{} {
	return p.pool.Stats()
}

// Resize 调整池大小（新方法）
func (p *ResourcePool) Resize(newMaxSize int) error {
	return p.pool.Resize(newMaxSize)
}
