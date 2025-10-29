package webrtc_vad

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"xiaozhi-server-go/internal/domain/vad/inter"
)

// PoolConfig 池配置
type PoolConfig struct {
	MaxSize     int           // 最大池大小
	MinSize     int           // 最小池大小
	MaxIdleTime time.Duration // 最大空闲时间
}

// DefaultPoolConfig 默认池配置
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxSize:     10,
		MinSize:     1,
		MaxIdleTime: 5 * time.Minute,
	}
}

// WebRTCVADPool WebRTC VAD 池
type WebRTCVADPool struct {
	config     PoolConfig
	vadConfig  inter.VADConfig
	pool       chan *WebRTCVAD
	created    atomic.Int64
	inUse      atomic.Int64
	mu         sync.RWMutex
	closed     atomic.Bool
	cleanupTicker *time.Ticker
}

// NewWebRTCVADPool 创建 WebRTC VAD 池
func NewWebRTCVADPool(vadConfig inter.VADConfig, poolConfig PoolConfig) (*WebRTCVADPool, error) {
	if poolConfig.MaxSize <= 0 {
		poolConfig.MaxSize = DefaultPoolConfig().MaxSize
	}
	if poolConfig.MinSize < 0 {
		poolConfig.MinSize = DefaultPoolConfig().MinSize
	}
	if poolConfig.MaxIdleTime <= 0 {
		poolConfig.MaxIdleTime = DefaultPoolConfig().MaxIdleTime
	}

	pool := &WebRTCVADPool{
		config:    poolConfig,
		vadConfig: vadConfig,
		pool:      make(chan *WebRTCVAD, poolConfig.MaxSize),
	}

	// 预创建最小数量的实例
	for i := 0; i < poolConfig.MinSize; i++ {
		vad, err := NewWebRTCVAD(vadConfig)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("failed to create initial VAD instance: %w", err)
		}
		pool.pool <- vad.(*WebRTCVAD)
		pool.created.Add(1)
	}

	// 启动清理协程
	pool.cleanupTicker = time.NewTicker(poolConfig.MaxIdleTime)
	go pool.cleanup()

	return pool, nil
}

// AcquireVAD 获取 VAD 实例
func (p *WebRTCVADPool) AcquireVAD() (inter.VADProvider, error) {
	if p.closed.Load() {
		return nil, fmt.Errorf("pool is closed")
	}

	// 尝试从池中获取
	select {
	case vad := <-p.pool:
		p.inUse.Add(1)
		return vad, nil
	default:
		// 池为空，创建新实例
		if p.created.Load() < int64(p.config.MaxSize) {
			vad, err := NewWebRTCVAD(p.vadConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to create VAD instance: %w", err)
			}
			p.created.Add(1)
			p.inUse.Add(1)
			return vad, nil
		}
		// 达到最大大小，阻塞等待
		vad := <-p.pool
		p.inUse.Add(1)
		return vad, nil
	}
}

// ReleaseVAD 释放 VAD 实例
func (p *WebRTCVADPool) ReleaseVAD(vad inter.VADProvider) error {
	if p.closed.Load() {
		return vad.Close()
	}

	webRTCVAD, ok := vad.(*WebRTCVAD)
	if !ok {
		return fmt.Errorf("invalid VAD type")
	}

	// 重置实例状态
	webRTCVAD.Reset()

	select {
	case p.pool <- webRTCVAD:
		p.inUse.Add(-1)
		return nil
	default:
		// 池已满，关闭实例
		p.inUse.Add(-1)
		return vad.Close()
	}
}

// Close 关闭池
func (p *WebRTCVADPool) Close() error {
	if !p.closed.CompareAndSwap(false, true) {
		return nil
	}

	p.cleanupTicker.Stop()

	close(p.pool)

	// 关闭所有实例
	for vad := range p.pool {
		vad.Close()
	}

	return nil
}

// Stats 获取池统计信息
func (p *WebRTCVADPool) Stats() (created, inUse, available int64) {
	created = p.created.Load()
	inUse = p.inUse.Load()
	available = created - inUse
	return
}

// cleanup 清理过期实例
func (p *WebRTCVADPool) cleanup() {
	for range p.cleanupTicker.C {
		if p.closed.Load() {
			return
		}

		p.mu.Lock()
		// 检查池中的实例是否过期
		tempPool := make([]*WebRTCVAD, 0, len(p.pool))
		for len(p.pool) > 0 {
			select {
			case vad := <-p.pool:
				if time.Since(vad.lastUsed) > p.config.MaxIdleTime && p.created.Load() > int64(p.config.MinSize) {
					// 实例过期且超过最小大小，关闭它
					vad.Close()
					p.created.Add(-1)
				} else {
					tempPool = append(tempPool, vad)
				}
			default:
				break
			}
		}

		// 放回池中
		for _, vad := range tempPool {
			select {
			case p.pool <- vad:
			default:
				// 池已满，关闭实例
				vad.Close()
				p.created.Add(-1)
			}
		}
		p.mu.Unlock()
	}
}