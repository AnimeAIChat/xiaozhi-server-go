package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
	"xiaozhi-server-go/src/core/utils"
)

// SessionKeys 会话密钥结构
type SessionKeys struct {
	Key       string    `json:"key"`        // AES密钥（十六进制）
	Nonce     string    `json:"nonce"`      // AES nonce（十六进制）
	SessionID string    `json:"session_id"` // 会话ID
	CreatedAt time.Time `json:"created_at"` // 创建时间
	ExpiresAt time.Time `json:"expires_at"` // 过期时间
}

// CryptoManager 加密管理器接口
type CryptoManager interface {
	GenerateSessionKeys(sessionID string) (*SessionKeys, error)
	GetSessionKeys(sessionID string) (*SessionKeys, error)
	RevokeSessionKeys(sessionID string) error
	CleanupExpiredKeys() error
	GetKeyStats() map[string]interface{}
}

// MemoryCryptoManager 内存加密管理器实现
type MemoryCryptoManager struct {
	keys   map[string]*SessionKeys
	mutex  sync.RWMutex
	logger *utils.Logger
	keyTTL time.Duration // 密钥有效期
}

// NewCryptoManager 创建加密管理器
func NewCryptoManager(logger *utils.Logger, keyTTL time.Duration) CryptoManager {
	if keyTTL <= 0 {
		keyTTL = 24 * time.Hour // 默认24小时有效期
	}

	return &MemoryCryptoManager{
		keys:   make(map[string]*SessionKeys),
		logger: logger,
		keyTTL: keyTTL,
	}
}

// generateAESKey 生成AES密钥（128位）
func generateAESKey() (string, error) {
	key := make([]byte, 16) // 128位密钥
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("生成AES密钥失败: %v", err)
	}
	return hex.EncodeToString(key), nil
}

// generateNonce 生成AES nonce（128位）
func generateNonce() (string, error) {
	nonce := make([]byte, 16) // 128位nonce（与ESP32期望的16字节一致）
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("生成AES nonce失败: %v", err)
	}
	return hex.EncodeToString(nonce), nil
}

// GenerateSessionKeys 生成会话密钥
func (cm *MemoryCryptoManager) GenerateSessionKeys(sessionID string) (*SessionKeys, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("会话ID不能为空")
	}

	// 生成密钥和nonce
	key, err := generateAESKey()
	if err != nil {
		return nil, err
	}

	nonce, err := generateNonce()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	sessionKeys := &SessionKeys{
		Key:       key,
		Nonce:     nonce,
		SessionID: sessionID,
		CreatedAt: now,
		ExpiresAt: now.Add(cm.keyTTL),
	}

	cm.mutex.Lock()
	cm.keys[sessionID] = sessionKeys
	cm.mutex.Unlock()

	cm.logger.Debug("生成会话密钥", map[string]interface{}{
		"session_id": sessionID,
		"expires_at": sessionKeys.ExpiresAt,
	})

	return sessionKeys, nil
}

// GetSessionKeys 获取会话密钥
func (cm *MemoryCryptoManager) GetSessionKeys(sessionID string) (*SessionKeys, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	keys, exists := cm.keys[sessionID]
	if !exists {
		return nil, fmt.Errorf("会话密钥不存在: %s", sessionID)
	}

	// 检查是否过期
	if time.Now().After(keys.ExpiresAt) {
		cm.mutex.RUnlock()
		cm.mutex.Lock()
		delete(cm.keys, sessionID)
		cm.mutex.Unlock()
		cm.mutex.RLock()
		return nil, fmt.Errorf("会话密钥已过期: %s", sessionID)
	}

	return keys, nil
}

// RevokeSessionKeys 撤销会话密钥
func (cm *MemoryCryptoManager) RevokeSessionKeys(sessionID string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	if _, exists := cm.keys[sessionID]; !exists {
		return fmt.Errorf("会话密钥不存在: %s", sessionID)
	}

	delete(cm.keys, sessionID)
	cm.logger.Info("撤销会话密钥", map[string]interface{}{
		"session_id": sessionID,
	})

	return nil
}

// CleanupExpiredKeys 清理过期密钥
func (cm *MemoryCryptoManager) CleanupExpiredKeys() error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	now := time.Now()
	expiredCount := 0

	for sessionID, keys := range cm.keys {
		if now.After(keys.ExpiresAt) {
			delete(cm.keys, sessionID)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		cm.logger.Info("清理过期密钥", map[string]interface{}{
			"expired_count": expiredCount,
		})
	}

	return nil
}

// GetKeyStats 获取密钥统计信息
func (cm *MemoryCryptoManager) GetKeyStats() map[string]interface{} {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	now := time.Now()
	totalKeys := len(cm.keys)
	expiredKeys := 0

	for _, keys := range cm.keys {
		if now.After(keys.ExpiresAt) {
			expiredKeys++
		}
	}

	return map[string]interface{}{
		"total_keys":   totalKeys,
		"active_keys":  totalKeys - expiredKeys,
		"expired_keys": expiredKeys,
		"key_ttl":      cm.keyTTL.String(),
	}
}
