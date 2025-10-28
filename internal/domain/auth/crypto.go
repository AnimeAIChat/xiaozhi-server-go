package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// SessionKeys encapsulates symmetric key material for a session.
type SessionKeys struct {
	Key       string    `json:"key"`
	Nonce     string    `json:"nonce"`
	SessionID string    `json:"session_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CryptoManager models the required crypto lifecycle methods.
type CryptoManager interface {
	GenerateSessionKeys(sessionID string) (*SessionKeys, error)
	GetSessionKeys(sessionID string) (*SessionKeys, error)
	RevokeSessionKeys(sessionID string) error
	CleanupExpiredKeys() error
	GetKeyStats() map[string]any
}

type memoryCryptoManager struct {
	keys   map[string]*SessionKeys
	mutex  sync.RWMutex
	ttl    time.Duration
	logger Logger
}

// NewMemoryCryptoManager builds an in-memory crypto manager with TTL enforcement.
func NewMemoryCryptoManager(logger Logger, ttl time.Duration) CryptoManager {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &memoryCryptoManager{
		keys:   make(map[string]*SessionKeys),
		ttl:    ttl,
		logger: logger,
	}
}

func generateHex(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (m *memoryCryptoManager) GenerateSessionKeys(sessionID string) (*SessionKeys, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session id must not be empty")
	}

	key, err := generateHex(16)
	if err != nil {
		return nil, fmt.Errorf("generate session key: %w", err)
	}
	nonce, err := generateHex(16)
	if err != nil {
		return nil, fmt.Errorf("generate session nonce: %w", err)
	}

	now := time.Now()
	keys := &SessionKeys{
		Key:       key,
		Nonce:     nonce,
		SessionID: sessionID,
		CreatedAt: now,
		ExpiresAt: now.Add(m.ttl),
	}

	m.mutex.Lock()
	m.keys[sessionID] = keys
	m.mutex.Unlock()

	if m.logger != nil {
		m.logger.Debug("session keys issued: %s", sessionID)
	}
	return keys, nil
}

func (m *memoryCryptoManager) GetSessionKeys(sessionID string) (*SessionKeys, error) {
	m.mutex.RLock()
	keys, ok := m.keys[sessionID]
	m.mutex.RUnlock()

	if !ok {
		return nil, fmt.Errorf("session keys not found: %s", sessionID)
	}
	if time.Now().After(keys.ExpiresAt) {
		m.mutex.Lock()
		delete(m.keys, sessionID)
		m.mutex.Unlock()
		return nil, fmt.Errorf("session keys expired: %s", sessionID)
	}
	return keys, nil
}

func (m *memoryCryptoManager) RevokeSessionKeys(sessionID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.keys, sessionID)
	if m.logger != nil {
		m.logger.Info("session keys revoked: %s", sessionID)
	}
	return nil
}

func (m *memoryCryptoManager) CleanupExpiredKeys() error {
	now := time.Now()

	m.mutex.Lock()
	for sessionID, keys := range m.keys {
		if now.After(keys.ExpiresAt) {
			delete(m.keys, sessionID)
		}
	}
	m.mutex.Unlock()
	return nil
}

func (m *memoryCryptoManager) GetKeyStats() map[string]any {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := map[string]any{
		"ttl":        m.ttl.String(),
		"total_keys": len(m.keys),
	}
	expired := 0
	now := time.Now()
	for _, keys := range m.keys {
		if now.After(keys.ExpiresAt) {
			expired++
		}
	}
	stats["expired_keys"] = expired
	stats["active_keys"] = len(m.keys) - expired
	return stats
}
