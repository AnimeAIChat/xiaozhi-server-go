package store

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// MemoryAuthStore 内存认证存储实现
type MemoryAuthStore struct {
	clients  map[string]*ClientInfo
	mutex    sync.RWMutex
	expiryHr int // 过期时间(小时)
}

// NewMemoryAuthStore 创建内存认证存储
func NewMemoryAuthStore(expiryHr int) *MemoryAuthStore {
	if expiryHr <= 0 {
		expiryHr = 24 // 默认24小时
	}

	store := &MemoryAuthStore{
		clients:  make(map[string]*ClientInfo),
		expiryHr: expiryHr,
	}

	// 启动定期清理过期数据的goroutine
	go store.periodicCleanup()

	return store
}

// StoreAuth 存储客户端认证信息
func (m *MemoryAuthStore) StoreAuth(
	clientID, username, password string,
	metadata map[string]interface{},
) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if clientID == "" {
		return fmt.Errorf("client_id不能为空")
	}

	// 解析用户名中的IP信息(base64编码的JSON)
	ip := ""
	deviceID := ""

	if username != "" {
		// 尝试解码base64
		if decoded, err := base64.StdEncoding.DecodeString(username); err == nil {
			var userInfo map[string]interface{}
			if json.Unmarshal(decoded, &userInfo) == nil {
				if ipValue, ok := userInfo["ip"].(string); ok {
					ip = ipValue
				}
			}
		}
	}

	// 从client_id中提取device_id (格式: CGID_test@@@device_id@@@uuid)
	if parts := strings.Split(clientID, "@@@"); len(parts) >= 2 {
		deviceID = parts[1]
	}

	now := time.Now()
	expiresAt := now.Add(time.Duration(m.expiryHr) * time.Hour)

	clientInfo := &ClientInfo{
		ClientID:  clientID,
		Username:  username,
		Password:  password,
		IP:        ip,
		DeviceID:  deviceID,
		CreatedAt: now,
		ExpiresAt: &expiresAt,
		Metadata:  metadata,
	}

	m.clients[clientID] = clientInfo

	return nil
}

// ValidateAuth 验证客户端认证信息
func (m *MemoryAuthStore) ValidateAuth(
	clientID, username, password string,
) (bool, *ClientInfo, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	clientInfo, exists := m.clients[clientID]
	if !exists {
		return false, nil, nil
	}

	// 检查是否过期
	if clientInfo.ExpiresAt != nil && time.Now().After(*clientInfo.ExpiresAt) {
		return false, nil, fmt.Errorf("认证信息已过期")
	}

	// 验证用户名和密码
	if clientInfo.Username != username || clientInfo.Password != password {
		return false, nil, nil
	}

	return true, clientInfo, nil
}

// GetClientInfo 获取客户端信息
func (m *MemoryAuthStore) GetClientInfo(clientID string) (*ClientInfo, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	clientInfo, exists := m.clients[clientID]
	if !exists {
		return nil, fmt.Errorf("客户端不存在: %s", clientID)
	}

	// 检查是否过期
	if clientInfo.ExpiresAt != nil && time.Now().After(*clientInfo.ExpiresAt) {
		return nil, fmt.Errorf("客户端认证已过期: %s", clientID)
	}

	// 返回副本以避免外部修改
	info := *clientInfo
	return &info, nil
}

// RemoveAuth 删除客户端认证信息
func (m *MemoryAuthStore) RemoveAuth(clientID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.clients, clientID)
	return nil
}

// ListClients 列出所有客户端ID
func (m *MemoryAuthStore) ListClients() ([]string, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var clients []string
	now := time.Now()

	for clientID, clientInfo := range m.clients {
		// 只返回未过期的客户端
		if clientInfo.ExpiresAt == nil || now.Before(*clientInfo.ExpiresAt) {
			clients = append(clients, clientID)
		}
	}

	return clients, nil
}

// CleanupExpired 清理过期认证
func (m *MemoryAuthStore) CleanupExpired() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	var expiredClients []string

	for clientID, clientInfo := range m.clients {
		if clientInfo.ExpiresAt != nil && now.After(*clientInfo.ExpiresAt) {
			expiredClients = append(expiredClients, clientID)
		}
	}

	for _, clientID := range expiredClients {
		delete(m.clients, clientID)
	}

	return nil
}

// Close 关闭存储连接
func (m *MemoryAuthStore) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 清空内存数据
	m.clients = make(map[string]*ClientInfo)
	return nil
}

// periodicCleanup 定期清理过期数据
func (m *MemoryAuthStore) periodicCleanup() {
	ticker := time.NewTicker(1 * time.Hour) // 每小时清理一次
	defer ticker.Stop()

	for range ticker.C {
		m.CleanupExpired()
	}
}

// GetStats 获取存储统计信息
func (m *MemoryAuthStore) GetStats() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	now := time.Now()
	total := len(m.clients)
	active := 0
	expired := 0

	for _, clientInfo := range m.clients {
		if clientInfo.ExpiresAt == nil || now.Before(*clientInfo.ExpiresAt) {
			active++
		} else {
			expired++
		}
	}

	return map[string]interface{}{
		"type":    "memory",
		"total":   total,
		"active":  active,
		"expired": expired,
	}
}
