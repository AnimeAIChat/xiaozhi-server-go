package util

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Resource 资源接口，所有被池管理的资源都需要实现此接口
type Resource interface {
	// Close 关闭资源
	Close() error
	// IsValid 检查资源是否有效
	IsValid() bool
}

// ResourceFactory 资源工厂接口，用于创建和验证资源
type ResourceFactory interface {
	// Create 创建新的资源实例
	Create() (Resource, error)
	// Validate 验证资源是否有效（可选，如果返回false，资源将被销毁）
	Validate(resource Resource) bool
	// Reset 重置资源状态（可选，用于资源复用前的清理）
	Reset(resource Resource) error
}

// PoolConfig 资源池配置
type PoolConfig struct {
	// MaxSize 最大资源数量
	MaxSize int
	// MinSize 最小资源数量（预创建）
	MinSize int
	// MaxIdle 最大空闲资源数量
	MaxIdle int
	// AcquireTimeout 获取资源超时时间
	AcquireTimeout time.Duration
	// IdleTimeout 资源空闲超时时间
	IdleTimeout time.Duration
	// ValidateOnBorrow 获取时是否验证资源
	ValidateOnBorrow bool
	// ValidateOnReturn 归还时是否验证资源
	ValidateOnReturn bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() *PoolConfig {
	return &PoolConfig{
		MaxSize:          10,
		MinSize:          1,
		MaxIdle:          5,
		AcquireTimeout:   30 * time.Second,
		IdleTimeout:      5 * time.Minute,
		ValidateOnBorrow: true,
		ValidateOnReturn: false,
	}
}

// pooledResource 池化资源包装器
type pooledResource struct {
	resource   Resource
	createTime time.Time
	lastUsed   time.Time
	inUse      bool
}

// ResourcePool 通用资源池
type ResourcePool struct {
	config  *PoolConfig
	factory ResourceFactory

	// 可用资源队列
	available chan *pooledResource
	// 所有资源映射（包括在用和可用的）
	resources map[Resource]*pooledResource
	// 读写锁
	mu sync.RWMutex
	// 关闭标志
	closed bool
	// 取消上下文
	ctx    context.Context
	cancel context.CancelFunc
	// 清理协程等待组
	cleanupWg sync.WaitGroup
}

// NewResourcePool 创建新的资源池
func NewResourcePool(config *PoolConfig, factory ResourceFactory) (*ResourcePool, error) {
	if config == nil {
		config = DefaultConfig()
	}
	if factory == nil {
		return nil, errors.New("factory cannot be nil")
	}
	if config.MaxSize <= 0 {
		return nil, errors.New("max size must be positive")
	}
	if config.MinSize < 0 {
		return nil, errors.New("min size cannot be negative")
	}
	if config.MinSize > config.MaxSize {
		return nil, errors.New("min size cannot be greater than max size")
	}

	ctx, cancel := context.WithCancel(context.Background())

	pool := &ResourcePool{
		config:    config,
		factory:   factory,
		available: make(chan *pooledResource, config.MaxSize),
		resources: make(map[Resource]*pooledResource),
		ctx:       ctx,
		cancel:    cancel,
	}

	// 预创建最小数量的资源
	if err := pool.preCreateResources(); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to pre-create resources: %w", err)
	}

	// 启动清理协程
	pool.startCleanupRoutine()

	return pool, nil
}

// preCreateResources 预创建资源
func (p *ResourcePool) preCreateResources() error {
	for i := 0; i < p.config.MinSize; i++ {
		resource, err := p.factory.Create()
		if err != nil {
			return fmt.Errorf("failed to create resource %d: %w", i, err)
		}

		pooled := &pooledResource{
			resource:   resource,
			createTime: time.Now(),
			lastUsed:   time.Now(),
			inUse:      false,
		}

		p.resources[resource] = pooled
		p.available <- pooled
	}
	return nil
}

// Acquire 获取资源
func (p *ResourcePool) Acquire() (Resource, error) {
	return p.AcquireWithTimeout(p.config.AcquireTimeout)
}

// AcquireWithTimeout 在指定超时时间内获取资源
func (p *ResourcePool) AcquireWithTimeout(timeout time.Duration) (Resource, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, errors.New("pool is closed")
	}
	p.mu.RUnlock()

	ctx, cancel := context.WithTimeout(p.ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("acquire timeout after %v", timeout)
		case pooled := <-p.available:
			// 验证资源有效性
			if p.config.ValidateOnBorrow && pooled.resource != nil {
				if !pooled.resource.IsValid() || !p.factory.Validate(pooled.resource) {
					// 资源无效，销毁并尝试创建新的
					p.destroyResource(pooled)
					if newResource, err := p.tryCreateResource(); err == nil {
						return newResource, nil
					}
					continue
				}
			}

			// 重置资源状态
			if err := p.factory.Reset(pooled.resource); err != nil {
				p.destroyResource(pooled)
				continue
			}

			// 标记为使用中
			p.mu.Lock()
			pooled.inUse = true
			pooled.lastUsed = time.Now()
			p.mu.Unlock()

			return pooled.resource, nil
		default:
			// 没有可用资源，尝试创建新的
			if resource, err := p.tryCreateResource(); err == nil {
				return resource, nil
			}
			// 创建失败，等待资源释放
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// tryCreateResource 尝试创建新资源
func (p *ResourcePool) tryCreateResource() (Resource, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.resources) >= p.config.MaxSize {
		return nil, errors.New("pool is full")
	}

	resource, err := p.factory.Create()
	if err != nil {
		return nil, err
	}

	pooled := &pooledResource{
		resource:   resource,
		createTime: time.Now(),
		lastUsed:   time.Now(),
		inUse:      true,
	}

	p.resources[resource] = pooled
	return resource, nil
}

// Release 释放资源回池
func (p *ResourcePool) Release(resource Resource) error {
	if resource == nil {
		return errors.New("resource cannot be nil")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return errors.New("pool is closed")
	}

	pooled, exists := p.resources[resource]
	if !exists {
		return errors.New("resource not managed by this pool")
	}

	if !pooled.inUse {
		return errors.New("resource is not in use")
	}

	// 验证资源有效性
	if p.config.ValidateOnReturn {
		if !resource.IsValid() || !p.factory.Validate(resource) {
			p.destroyResourceUnsafe(pooled)
			return nil
		}
	}

	// 检查是否超过最大空闲数量
	if len(p.available) >= p.config.MaxIdle {
		p.destroyResourceUnsafe(pooled)
		return nil
	}

	// 标记为可用
	pooled.inUse = false
	pooled.lastUsed = time.Now()

	// 尝试放回可用队列
	select {
	case p.available <- pooled:
		return nil
	default:
		// 队列已满，销毁资源
		p.destroyResourceUnsafe(pooled)
		return nil
	}
}

// destroyResource 销毁资源（带锁）
func (p *ResourcePool) destroyResource(pooled *pooledResource) {
	p.mu.Lock()
	p.destroyResourceUnsafe(pooled)
	p.mu.Unlock()
}

// destroyResourceUnsafe 销毁资源（不带锁）
func (p *ResourcePool) destroyResourceUnsafe(pooled *pooledResource) {
	if pooled.resource != nil {
		pooled.resource.Close()
		delete(p.resources, pooled.resource)
	}
}

// Stats 获取资源池统计信息
func (p *ResourcePool) Stats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	inUseCount := 0
	for _, pooled := range p.resources {
		if pooled.inUse {
			inUseCount++
		}
	}

	return map[string]interface{}{
		"total_resources":     len(p.resources),
		"available_resources": len(p.available),
		"in_use_resources":    inUseCount,
		"max_size":            p.config.MaxSize,
		"min_size":            p.config.MinSize,
		"max_idle":            p.config.MaxIdle,
		"is_closed":           p.closed,
	}
}

// Resize 调整池大小
func (p *ResourcePool) Resize(newMaxSize int) error {
	if newMaxSize <= 0 {
		return errors.New("new max size must be positive")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return errors.New("pool is closed")
	}

	oldMaxSize := p.config.MaxSize
	p.config.MaxSize = newMaxSize

	// 如果缩小池大小，需要移除多余的资源
	if newMaxSize < oldMaxSize {
		excess := len(p.resources) - newMaxSize
		for excess > 0 {
			select {
			case pooled := <-p.available:
				p.destroyResourceUnsafe(pooled)
				excess--
			default:
				// 没有更多可用资源可以移除
				break
			}
		}
	}

	return nil
}

// startCleanupRoutine 启动清理协程
func (p *ResourcePool) startCleanupRoutine() {
	if p.config.IdleTimeout <= 0 {
		return
	}

	p.cleanupWg.Add(1)
	go func() {
		defer p.cleanupWg.Done()
		ticker := time.NewTicker(p.config.IdleTimeout / 2)
		defer ticker.Stop()

		for {
			select {
			case <-p.ctx.Done():
				return
			case <-ticker.C:
				p.cleanupIdleResources()
			}
		}
	}()
}

// cleanupIdleResources 清理空闲超时的资源
func (p *ResourcePool) cleanupIdleResources() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	now := time.Now()
	var toRemove []*pooledResource

	// 检查可用队列中的空闲资源
	for {
		select {
		case pooled := <-p.available:
			if now.Sub(pooled.lastUsed) > p.config.IdleTimeout {
				toRemove = append(toRemove, pooled)
			} else {
				// 放回队列
				p.available <- pooled
				goto cleanup
			}
		default:
			goto cleanup
		}
	}

cleanup:
	// 销毁超时的资源
	for _, pooled := range toRemove {
		p.destroyResourceUnsafe(pooled)
	}
}

// Close 关闭资源池
func (p *ResourcePool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	// 取消上下文
	p.cancel()

	// 等待清理协程结束
	p.cleanupWg.Wait()

	// 关闭所有资源
	p.mu.Lock()
	defer p.mu.Unlock()

	// 清空可用队列
	close(p.available)
	for pooled := range p.available {
		p.destroyResourceUnsafe(pooled)
	}

	// 关闭所有资源
	for _, pooled := range p.resources {
		p.destroyResourceUnsafe(pooled)
	}

	return nil
}