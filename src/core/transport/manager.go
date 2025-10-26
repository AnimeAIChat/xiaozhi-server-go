package transport

import (
	"context"
	"sync"
	"time"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/core/utils"
)

// TransportManager 传输管理器
type TransportManager struct {
	transports map[string]Transport
	logger     *utils.Logger
	config     *configs.Config
	mu         sync.RWMutex
}

// NewTransportManager 创建新的传输管理器
func NewTransportManager(config *configs.Config, logger *utils.Logger) *TransportManager {
	return &TransportManager{
		transports: make(map[string]Transport),
		logger:     logger,
		config:     config,
	}
}

// RegisterTransport 注册传输层
func (m *TransportManager) RegisterTransport(name string, transport Transport) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transports[name] = transport
	m.logger.Debug("注册传输层: %s (%s)", name, transport.GetType())
}

// StartAll 启动所有传输层
func (m *TransportManager) StartAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, transport := range m.transports {
		// m.logger.Info(fmt.Sprintf("启动传输层: %s", name))

		// 为每个传输层启动独立的goroutine
		go func(name string, transport Transport) {
			if err := transport.Start(ctx); err != nil {
				m.logger.Error("传输层 %s 运行失败: %v", name, err)
			}
		}(name, transport)
	}

	m.StartTicker(ctx)

	return nil
}

func (m *TransportManager) StartTicker(ctx context.Context) {
	// 设置定时器，打印各个传输层的状态信息
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				clientCnt := 0
				sessionCnt := 0
				for _, transport := range m.transports {
					c, s := transport.GetActiveConnectionCount()
					clientCnt += c
					sessionCnt += s
				}
				//m.logger.Info("当前活跃连接数: %d, 当前活跃会话数: %d", clientCnt, sessionCnt)
				systemMemoryUse, _ := utils.GetSystemMemoryUsage()
				systemCPUUse, _ := utils.GetSystemCPUUsage()
				database.UpdateServerStatus(systemMemoryUse, systemCPUUse, clientCnt, sessionCnt)
			}
		}
	}()
}

// StopAll 停止所有传输层
func (m *TransportManager) StopAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var lastErr error
	for name, transport := range m.transports {
		if err := transport.Stop(); err != nil {
			m.logger.Error("停止传输层 %s 失败: %v", name, err)
			lastErr = err
		}
	}
	return lastErr
}

// GetTransport 获取指定名称的传输层
func (m *TransportManager) GetTransport(name string) Transport {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.transports[name]
}
