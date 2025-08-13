package transport

import (
	"context"
	"sync"
	"xiaozhi-server-go/src/configs"
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
	return nil
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

// GetStats 获取所有传输层的统计信息
func (m *TransportManager) GetStats() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]int)
	for name, transport := range m.transports {
		stats[name] = transport.GetActiveConnectionCount()
	}
	return stats
}

// GetTransport 获取指定名称的传输层
func (m *TransportManager) GetTransport(name string) Transport {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.transports[name]
}

// GetTotalConnections 获取所有传输层的总连接数
func (m *TransportManager) GetTotalConnections() int {
	stats := m.GetStats()
	total := 0
	for _, count := range stats {
		total += count
	}
	return total
}
