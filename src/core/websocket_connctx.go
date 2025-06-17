package core

import (
	"context"
	"fmt"
	"sync/atomic"
	"xiaozhi-server-go/src/core/pool"
	"xiaozhi-server-go/src/core/utils"
)

// ConnectionContext 连接上下文，用于跟踪资源分配
type ConnectionContext struct {
	handler     *ConnectionHandler
	providerSet *pool.ProviderSet
	poolManager *pool.PoolManager
	clientID    string
	logger      *utils.Logger
	conn        Connection
	ctx         context.Context
	cancel      context.CancelFunc
	closed      int32 // 原子操作标志，0=活跃，1=已关闭
}

// NewConnectionContext 创建新的连接上下文
func NewConnectionContext(handler *ConnectionHandler, providerSet *pool.ProviderSet,
	poolManager *pool.PoolManager, clientID string, logger *utils.Logger, conn Connection,
	ctx context.Context, cancel context.CancelFunc) *ConnectionContext {

	return &ConnectionContext{
		handler:     handler,
		providerSet: providerSet,
		poolManager: poolManager,
		clientID:    clientID,
		logger:      logger,
		conn:        conn,
		ctx:         ctx,
		cancel:      cancel,
		closed:      0,
	}
}

// IsActive 检查连接是否仍然活跃
func (c *ConnectionContext) IsActive() bool {
	return atomic.LoadInt32(&c.closed) == 0
}

// GetContext 获取上下文（用于取消操作）
func (c *ConnectionContext) GetContext() context.Context {
	return c.ctx
}

// CreateSafeCallback 创建安全的回调函数
func (c *ConnectionContext) CreateSafeCallback() func(func(*ConnectionHandler)) func() {
	return func(callback func(*ConnectionHandler)) func() {
		return func() {
			// 检查连接是否仍然活跃
			if !c.IsActive() {
				c.logger.Info(fmt.Sprintf("客户端 %s 连接已关闭，跳过回调", c.clientID))
				return
			}

			// 检查上下文是否已取消
			select {
			case <-c.ctx.Done():
				c.logger.Info(fmt.Sprintf("客户端 %s 上下文已取消，跳过回调", c.clientID))
				return
			default:
			}

			// 执行回调
			if c.handler != nil {
				callback(c.handler)
			}
		}
	}
}

// Close 关闭连接并归还资源
func (c *ConnectionContext) Close() error {
	// 使用原子操作标记为已关闭
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil // 已经关闭过了
	}

	// 取消上下文，通知所有相关操作停止
	c.cancel()

	var errs []error

	// 先关闭连接处理器
	if c.handler != nil {
		c.handler.Close()
	}

	// 关闭WebSocket连接
	if c.conn != nil {
		c.conn.Close()
	}

	// 归还资源到池中
	if c.providerSet != nil && c.poolManager != nil {
		if err := c.poolManager.ReturnProviderSet(c.providerSet); err != nil {
			errs = append(errs, fmt.Errorf("归还资源失败: %v", err))
			c.logger.Error(fmt.Sprintf("客户端 %s 归还资源失败: %v", c.clientID, err))
		} else {
			c.logger.Info(fmt.Sprintf("客户端 %s 资源已成功归还到池中", c.clientID))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("关闭连接时发生错误: %v", errs)
	}
	return nil
}
