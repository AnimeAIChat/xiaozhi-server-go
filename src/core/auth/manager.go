package auth

import (
	"fmt"
	"sync"
	"time"
	"xiaozhi-server-go/src/core/auth/store"
	"xiaozhi-server-go/src/core/utils"
)

// AuthManager 认证管理器
type AuthManager struct {
	store         store.AuthStore
	cryptoManager CryptoManager
	logger        *utils.Logger
	mutex         sync.RWMutex
}

// NewAuthManager 创建认证管理器
func NewAuthManager(storeConfig *store.StoreConfig, logger *utils.Logger) (*AuthManager, error) {
	if storeConfig == nil {
		storeConfig = store.GetDefaultStoreConfig()
	}

	// 验证配置
	if err := store.ValidateStoreConfig(storeConfig); err != nil {
		return nil, fmt.Errorf("认证存储配置验证失败: %v", err)
	}

	// 创建存储实例
	authStore, err := store.CreateAuthStore(storeConfig)
	if err != nil {
		return nil, fmt.Errorf("创建认证存储失败: %v", err)
	}

	// 创建加密管理器
	cryptoManager := NewCryptoManager(logger, 24*time.Hour) // 密钥24小时有效期

	manager := &AuthManager{
		store:         authStore,
		cryptoManager: cryptoManager,
		logger:        logger,
	}

	manager.logger.Info("认证管理器初始化成功", map[string]interface{}{
		"store_type": storeConfig.Type,
		"expiry_hr":  storeConfig.ExpiryHr,
	})

	return manager, nil
}

// RegisterClient 注册客户端认证信息
func (am *AuthManager) RegisterClient(
	clientID, username, password string,
	metadata map[string]interface{},
) error {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	if clientID == "" {
		return fmt.Errorf("客户端ID不能为空")
	}

	// 存储认证信息
	err := am.store.StoreAuth(clientID, username, password, metadata)
	if err != nil {
		am.logger.Error("注册客户端认证信息失败", map[string]interface{}{
			"client_id": clientID,
			"error":     err.Error(),
		})
		return err
	}
	return nil
}

// AuthenticateClient 验证客户端认证
func (am *AuthManager) AuthenticateClient(
	clientID, username, password string,
) (bool, *store.ClientInfo, error) {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	valid, clientInfo, err := am.store.ValidateAuth(clientID, username, password)
	if err != nil {
		am.logger.Error("客户端认证验证失败", map[string]interface{}{
			"client_id": clientID,
			"error":     err.Error(),
		})
		return false, nil, err
	}

	if !valid {
		am.logger.Debug("客户端认证失败", map[string]interface{}{
			"client_id": clientID,
			"username":  username,
		})
		return false, nil, nil
	}

	am.logger.Debug("客户端认证成功", map[string]interface{}{
		"client_id": clientID,
		"username":  username,
		"ip":        clientInfo.IP,
		"device_id": clientInfo.DeviceID,
	})

	return true, clientInfo, nil
}

// GetClientInfo 获取客户端信息
func (am *AuthManager) GetClientInfo(clientID string) (*store.ClientInfo, error) {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	return am.store.GetClientInfo(clientID)
}

// RemoveClient 移除客户端认证信息
func (am *AuthManager) RemoveClient(clientID string) error {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	err := am.store.RemoveAuth(clientID)
	if err != nil {
		am.logger.Error("移除客户端认证信息失败", map[string]interface{}{
			"client_id": clientID,
			"error":     err.Error(),
		})
		return err
	}

	am.logger.Info("客户端认证信息已移除", map[string]interface{}{
		"client_id": clientID,
	})

	return nil
}

// ListClients 列出所有已认证的客户端
func (am *AuthManager) ListClients() ([]string, error) {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	return am.store.ListClients()
}

// CleanupExpired 清理过期的认证信息
func (am *AuthManager) CleanupExpired() error {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	err := am.store.CleanupExpired()
	if err != nil {
		am.logger.Error("清理过期认证信息失败", map[string]interface{}{
			"error": err.Error(),
		})
		return err
	}

	am.logger.Info("过期认证信息清理完成")
	return nil
}

// GetStats 获取认证存储统计信息
func (am *AuthManager) GetStats() map[string]interface{} {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	// 检查存储是否有统计方法
	if memStore, ok := am.store.(*store.MemoryAuthStore); ok {
		return memStore.GetStats()
	}

	// 对于其他存储类型，返回基本信息
	clients, _ := am.store.ListClients()
	return map[string]interface{}{
		"type":   "unknown",
		"active": len(clients),
	}
}

// Close 关闭认证管理器
func (am *AuthManager) Close() error {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	if am.store != nil {
		err := am.store.Close()
		if err != nil {
			am.logger.Error("关闭认证存储失败", map[string]interface{}{
				"error": err.Error(),
			})
			return err
		}
	}

	am.logger.Info("认证管理器已关闭")
	return nil
}

// 加密相关方法

// GenerateSessionKeys 生成会话密钥
func (am *AuthManager) GenerateSessionKeys(sessionID string) (*SessionKeys, error) {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	return am.cryptoManager.GenerateSessionKeys(sessionID)
}

// GetSessionKeys 获取会话密钥
func (am *AuthManager) GetSessionKeys(sessionID string) (*SessionKeys, error) {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	return am.cryptoManager.GetSessionKeys(sessionID)
}

// RevokeSessionKeys 撤销会话密钥
func (am *AuthManager) RevokeSessionKeys(sessionID string) error {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	return am.cryptoManager.RevokeSessionKeys(sessionID)
}

// CleanupExpiredKeys 清理过期密钥
func (am *AuthManager) CleanupExpiredKeys() error {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	return am.cryptoManager.CleanupExpiredKeys()
}

// GetCryptoStats 获取加密统计信息
func (am *AuthManager) GetCryptoStats() map[string]interface{} {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	return am.cryptoManager.GetKeyStats()
}
