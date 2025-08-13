package store

import (
	"time"
)

// ClientInfo 客户端认证信息结构
type ClientInfo struct {
	ClientID  string                 `json:"client_id"`
	Username  string                 `json:"username"`
	Password  string                 `json:"password"`
	IP        string                 `json:"ip"`
	DeviceID  string                 `json:"device_id"`
	CreatedAt time.Time              `json:"created_at"`
	ExpiresAt *time.Time             `json:"expires_at,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// AuthStore 认证存储接口
type AuthStore interface {
	// StoreAuth 存储客户端认证信息
	StoreAuth(clientID, username, password string, metadata map[string]interface{}) error

	// ValidateAuth 验证客户端认证信息
	ValidateAuth(clientID, username, password string) (bool, *ClientInfo, error)

	// GetClientInfo 获取客户端信息
	GetClientInfo(clientID string) (*ClientInfo, error)

	// RemoveAuth 删除客户端认证信息
	RemoveAuth(clientID string) error

	// ListClients 列出所有客户端ID
	ListClients() ([]string, error)

	// CleanupExpired 清理过期认证
	CleanupExpired() error

	// Close 关闭存储连接
	Close() error
}

// StoreConfig 存储配置
type StoreConfig struct {
	Type     string                 `yaml:"type"`   // memory/file/redis
	Config   map[string]interface{} `yaml:"config"` // 具体存储的配置
	ExpiryHr int                    `yaml:"expiry"` // 过期时间(小时)
}
