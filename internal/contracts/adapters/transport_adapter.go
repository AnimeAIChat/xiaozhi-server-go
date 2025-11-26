package adapters

import (
	"context"

	"xiaozhi-server-go/internal/platform/config"
	"xiaozhi-server-go/src/core/utils"
)

// TransportAdapter 传输层适配器
// 在过渡期间提供与旧传输系统的兼容性
type TransportAdapter struct {
	config      *config.Config
	logger      *utils.Logger
	legacyAdapter *LegacyPoolManagerAdapter
}

// NewTransportAdapter 创建传输层适配器
func NewTransportAdapter(cfg *config.Config, logger *utils.Logger, legacyAdapter *LegacyPoolManagerAdapter) *TransportAdapter {
	return &TransportAdapter{
		config:       cfg,
		logger:       logger,
		legacyAdapter: legacyAdapter,
	}
}

// StartTransportServer 启动传输服务器（占位符实现）
func (ta *TransportAdapter) StartTransportServer(ctx context.Context, authManager interface{}, domainMCPManager interface{}) error {
	if ta.logger != nil {
		ta.logger.InfoTag("传输适配器", "正在启动传输服务器...")
	}

	// TODO: 在第二阶段实现实际的传输服务器启动逻辑
	// 现在先返回成功，避免阻塞系统启动

	if ta.logger != nil {
		ta.logger.InfoTag("传输适配器", "传输服务器启动完成（占位符实现）")
	}

	return nil
}

// StopTransportServer 停止传输服务器
func (ta *TransportAdapter) StopTransportServer() error {
	if ta.logger != nil {
		ta.logger.InfoTag("传输适配器", "正在停止传输服务器...")
	}

	// TODO: 实现传输服务器停止逻辑

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