package transport

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/core"
	"xiaozhi-server-go/src/core/pool"
	"xiaozhi-server-go/src/core/utils"
	"xiaozhi-server-go/src/task"
)

// ConnectionContextAdapter 连接上下文适配器，完全兼容现有的ConnectionContext逻辑
type ConnectionContextAdapter struct {
	handler     *core.ConnectionHandler
	providerSet *pool.ProviderSet
	poolManager *pool.PoolManager
	clientID    string
	logger      *utils.Logger
	conn        Connection
	ctx         context.Context
	cancel      context.CancelFunc
	closed      int32 // 原子操作标志，0=活跃，1=已关闭
}

// NewConnectionContextAdapter 创建新的连接上下文适配器
func NewConnectionContextAdapter(
	conn Connection,
	config *configs.Config,
	providerSet *pool.ProviderSet,
	poolManager *pool.PoolManager,
	taskMgr *task.TaskManager,
	logger *utils.Logger,
	req *http.Request,
) *ConnectionContextAdapter {
	clientID := conn.GetID()
	connCtx, connCancel := context.WithCancel(context.Background())

	// 创建ConnectionHandler
	handler := core.NewConnectionHandler(config, providerSet, logger, req, connCtx)

	adapter := &ConnectionContextAdapter{
		handler:     handler,
		providerSet: providerSet,
		poolManager: poolManager,
		clientID:    clientID,
		logger:      logger,
		conn:        conn,
		ctx:         connCtx,
		cancel:      connCancel,
		closed:      0,
	}

	// 设置TaskManager和回调
	handler.SetTaskCallback(adapter.CreateSafeCallback())

	return adapter
}

// Handle 实现ConnectionHandler接口的Handle方法
func (a *ConnectionContextAdapter) Handle() {
	// 适配原有的Handle方法，传入适配的连接
	a.handler.Handle(a.conn)
	a.logger.Info(fmt.Sprintf("客户端 %s 连接处理完成", a.clientID))
}

// Close 实现ConnectionHandler接口的Close方法，完全兼容原有逻辑
func (a *ConnectionContextAdapter) Close() {
	// 使用原子操作标记为已关闭
	if !atomic.CompareAndSwapInt32(&a.closed, 0, 1) {
		a.logger.Info(fmt.Sprintf("客户端 %s 连接已关闭，跳过重复关闭", a.clientID))
		return // 已经关闭过了
	}

	// 取消上下文，通知所有相关操作停止
	a.cancel()

	// 先关闭连接处理器
	if a.handler != nil {
		a.handler.Close()
	}

	// 关闭连接
	if a.conn != nil {
		a.conn.Close()
	}

	// 归还资源到池中
	if a.providerSet != nil && a.poolManager != nil {
		if err := a.poolManager.ReturnProviderSet(a.providerSet); err != nil {
			a.logger.Error("客户端 %s 归还资源失败: %v", a.clientID, err)
		} else {
			a.logger.Info("客户端 %s 资源已成功归还到池中", a.clientID)
		}
	}
}

// GetSessionID 实现ConnectionHandler接口的GetSessionID方法
func (a *ConnectionContextAdapter) GetSessionID() string {
	return a.clientID
}

// IsActive 检查连接是否仍然活跃
func (a *ConnectionContextAdapter) IsActive() bool {
	return atomic.LoadInt32(&a.closed) == 0
}

// GetContext 获取上下文（用于取消操作）
func (a *ConnectionContextAdapter) GetContext() context.Context {
	return a.ctx
}

// GetConnectionHandler 获取内部的ConnectionHandler
func (a *ConnectionContextAdapter) GetConnectionHandler() *core.ConnectionHandler {
	return a.handler
}

// CreateSafeCallback 创建安全的回调函数，完全兼容原有逻辑
func (a *ConnectionContextAdapter) CreateSafeCallback() func(func(*core.ConnectionHandler)) func() {
	return func(callback func(*core.ConnectionHandler)) func() {
		return func() {
			// 检查连接是否仍然活跃
			if !a.IsActive() {
				a.logger.Info(fmt.Sprintf("客户端 %s 连接已关闭，跳过回调", a.clientID))
				return
			}

			// 检查上下文是否已取消
			select {
			case <-a.ctx.Done():
				a.logger.Info(fmt.Sprintf("客户端 %s 上下文已取消，跳过回调", a.clientID))
				return
			default:
			}

			// 执行回调
			if a.handler != nil {
				callback(a.handler)
			}
		}
	}
}

// DefaultConnectionHandlerFactory 默认连接处理器工厂
type DefaultConnectionHandlerFactory struct {
	config      *configs.Config
	poolManager *pool.PoolManager
	taskMgr     *task.TaskManager
	logger      *utils.Logger
}

// NewDefaultConnectionHandlerFactory 创建默认连接处理器工厂
func NewDefaultConnectionHandlerFactory(
	config *configs.Config,
	poolManager *pool.PoolManager,
	taskMgr *task.TaskManager,
	logger *utils.Logger,
) *DefaultConnectionHandlerFactory {
	return &DefaultConnectionHandlerFactory{
		config:      config,
		poolManager: poolManager,
		taskMgr:     taskMgr,
		logger:      logger,
	}
}

// CreateHandler 实现ConnectionHandlerFactory接口
func (f *DefaultConnectionHandlerFactory) CreateHandler(
	conn Connection,
	req *http.Request,
) ConnectionHandler {
	// 从资源池获取提供者集合
	providerSet, err := f.poolManager.GetProviderSet()
	if err != nil {
		f.logger.Error(fmt.Sprintf("获取提供者集合失败: %v", err))
		return nil
	}

	// 创建连接上下文适配器
	adapter := NewConnectionContextAdapter(
		conn,
		f.config,
		providerSet,
		f.poolManager,
		f.taskMgr,
		f.logger,
		req,
	)

	return adapter
}
