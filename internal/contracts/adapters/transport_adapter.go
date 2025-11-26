package adapters

import (
	"context"
	"fmt"

	"xiaozhi-server-go/internal/platform/config"
	websockettransport "xiaozhi-server-go/internal/core/transport/websocket"
	"xiaozhi-server-go/internal/core/transport"
	"xiaozhi-server-go/internal/utils"
	"xiaozhi-server-go/internal/core/pool"
	"xiaozhi-server-go/internal/domain/task"
)

// TransportAdapter 传输层适配器
// 在过渡期间提供与旧传输系统的兼容性
type TransportAdapter struct {
	config       *config.Config
	logger       *utils.Logger
	legacyAdapter *LegacyPoolManagerAdapter

	// WebSocket 服务器组件
	wsTransport  *websockettransport.WebSocketTransport
	poolManager  *pool.PoolManager
	taskMgr      *task.TaskManager
}

// NewTransportAdapter 创建传输层适配器
func NewTransportAdapter(cfg *config.Config, logger *utils.Logger, legacyAdapter *LegacyPoolManagerAdapter) *TransportAdapter {
	adapter := &TransportAdapter{
		config:        cfg,
		logger:        logger,
		legacyAdapter: legacyAdapter,
	}

	// 初始化WebSocket传输
	if cfg.Transport.WebSocket.Enabled {
		// 创建池管理器
		poolManager, err := pool.NewPoolManager(cfg, logger)
		if err != nil {
			if logger != nil {
				logger.ErrorTag("传输适配器", "创建池管理器失败: %v", err)
			}
			return adapter
		}
		adapter.poolManager = poolManager

		// 创建任务管理器
		taskConfig := task.ResourceConfig{
			MaxWorkers:        10,
			MaxTasksPerClient: 5,
		}
		taskMgr := task.NewTaskManager(taskConfig)
		adapter.taskMgr = taskMgr

		// 创建WebSocket传输
		adapter.wsTransport = websockettransport.NewWebSocketTransport(cfg, logger)

		// 设置连接处理器工厂
		connFactory := transport.NewDefaultConnectionHandlerFactory(cfg, poolManager, taskMgr, logger)
		adapter.wsTransport.SetConnectionHandler(connFactory)

		if logger != nil {
			logger.InfoTag("传输适配器", "WebSocket传输已初始化，已设置连接处理器工厂和池管理器")
		}
	} else {
		if logger != nil {
			logger.InfoTag("传输适配器", "WebSocket服务已禁用")
		}
	}

	return adapter
}

// StartTransportServer 启动传输服务器
func (ta *TransportAdapter) StartTransportServer(ctx context.Context, authManager interface{}, domainMCPManager interface{}) error {
	if ta.logger != nil {
		ta.logger.InfoTag("传输适配器", "正在启动传输服务器...")
	}

	// 如果WebSocket服务被禁用，直接返回成功
	if !ta.config.Transport.WebSocket.Enabled {
		if ta.logger != nil {
			ta.logger.InfoTag("传输适配器", "WebSocket服务已禁用，跳过启动")
		}
		return nil
	}

	// 检查WebSocket传输是否已初始化
	if ta.wsTransport == nil {
		return fmt.Errorf("WebSocket传输未初始化")
	}

	// 启动WebSocket传输
	go func() {
		if err := ta.wsTransport.Start(ctx); err != nil {
			if ta.logger != nil {
				ta.logger.ErrorTag("传输适配器", "WebSocket传输启动失败: %v", err)
			}
		} else {
			if ta.logger != nil {
				ta.logger.InfoTag("传输适配器", "WebSocket传输启动成功")
			}
		}
	}()

	if ta.logger != nil {
		ta.logger.InfoTag("传输适配器", "传输服务器启动完成")
	}

	return nil
}

// StopTransportServer 停止传输服务器
func (ta *TransportAdapter) StopTransportServer() error {
	if ta.logger != nil {
		ta.logger.InfoTag("传输适配器", "正在停止传输服务器...")
	}

	// 如果WebSocket服务被禁用或未初始化，直接返回成功
	if !ta.config.Transport.WebSocket.Enabled || ta.wsTransport == nil {
		if ta.logger != nil {
			ta.logger.InfoTag("传输适配器", "WebSocket服务未启用，跳过停止")
		}
		return nil
	}

	// 停止WebSocket传输
	if err := ta.wsTransport.Stop(); err != nil {
		if ta.logger != nil {
			ta.logger.ErrorTag("传输适配器", "WebSocket传输停止失败: %v", err)
		}
		return err
	}

	if ta.logger != nil {
		ta.logger.InfoTag("传输适配器", "传输服务器已停止")
	}

	return nil
}

// TransportManager 传输管理器接口
type TransportManager interface {
	Start(ctx context.Context) error
	Stop() error
	GetStats() map[string]interface{}
}

// MockTransportManager 模拟传输管理器
type MockTransportManager struct {
	logger *utils.Logger
}

// NewMockTransportManager 创建模拟传输管理器
func NewMockTransportManager(logger *utils.Logger) *MockTransportManager {
	return &MockTransportManager{
		logger: logger,
	}
}

// Start 启动传输管理器
func (m *MockTransportManager) Start(ctx context.Context) error {
	if m.logger != nil {
		m.logger.InfoTag("模拟传输管理器", "传输管理器启动（占位符实现）")
	}
	return nil
}

// Stop 停止传输管理器
func (m *MockTransportManager) Stop() error {
	if m.logger != nil {
		m.logger.InfoTag("模拟传输管理器", "传输管理器停止")
	}
	return nil
}

// GetStats 获取统计信息
func (m *MockTransportManager) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"active_connections": 0,
		"total_requests":     0,
		"uptime_seconds":     0,
	}
}