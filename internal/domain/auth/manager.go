package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"xiaozhi-server-go/internal/domain/auth/model"
	"xiaozhi-server-go/internal/domain/auth/store"
)

type (
	// ClientInfo re-exports the shared auth entity for callers.
	ClientInfo = model.ClientInfo
	// Logger re-exports the logging interface used across the domain.
	Logger = model.Logger
)

const (
	defaultCleanupInterval = 10 * time.Minute
	minCleanupInterval     = 30 * time.Second
)

// Options encapsulates the dependencies required to construct a Manager.
type Options struct {
	Store           store.Store
	Logger          Logger
	Crypto          CryptoManager
	SessionTTL      time.Duration
	CleanupInterval time.Duration
}

// Manager coordinates authentication storage and crypto lifecycle.
type Manager struct {
	store      store.Store
	logger     Logger
	crypto     CryptoManager
	sessionTTL time.Duration

	cleanupInterval time.Duration
	cleanupStop     chan struct{}
	cleanupOnce     sync.Once
	mu              sync.RWMutex
}

// AuthManager preserves the legacy exported type name.
type AuthManager = Manager

// NewManager wires a Manager using the supplied options.
func NewManager(opts Options) (*Manager, error) {
	if opts.Store == nil {
		return nil, errors.New("auth manager requires a store")
	}
	if opts.Logger == nil {
		return nil, errors.New("auth manager requires a logger")
	}
	if opts.Crypto == nil {
		opts.Logger.Warn("auth manager using in-memory crypto backend")
		opts.Crypto = NewMemoryCryptoManager(opts.Logger, 24*time.Hour)
	}
	sessionTTL := opts.SessionTTL
	if sessionTTL <= 0 {
		sessionTTL = 24 * time.Hour
	}
	cleanupInterval := opts.CleanupInterval
	if cleanupInterval <= 0 {
		cleanupInterval = defaultCleanupInterval
	} else if cleanupInterval < minCleanupInterval {
		opts.Logger.Warn(
			"cleanup interval too small, adjusting to minimum",
			minCleanupInterval,
		)
		cleanupInterval = minCleanupInterval
	}
	mgr := &Manager{
		store:           opts.Store,
		logger:          opts.Logger,
		crypto:          opts.Crypto,
		sessionTTL:      sessionTTL,
		cleanupInterval: cleanupInterval,
		cleanupStop:     make(chan struct{}),
	}

	go mgr.runCleanup()
	return mgr, nil
}

func (m *Manager) runCleanup() {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := m.store.CleanupExpired(context.Background()); err != nil {
				m.logger.Warn("auth store cleanup failed: %v", err)
			}
			if err := m.crypto.CleanupExpiredKeys(); err != nil {
				m.logger.Warn("auth crypto cleanup failed: %v", err)
			}
		case <-m.cleanupStop:
			return
		}
	}
}

// RegisterClient persists credentials and metadata.
func (m *Manager) RegisterClient(ctx context.Context, info ClientInfo) error {
	if info.ClientID == "" {
		return fmt.Errorf("client id must not be empty")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	now := time.Now()
	info.CreatedAt = now
	if m.sessionTTL > 0 {
		expiresAt := now.Add(m.sessionTTL)
		info.ExpiresAt = &expiresAt
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.store.Store(ctx, info); err != nil {
		m.logger.Error("failed to register client: %s: %v", info.ClientID, err)
		return err
	}
	m.logger.Debug("registered auth client: %s", info.ClientID)
	return nil
}

// Authenticate verifies credentials and returns client context.
func (m *Manager) Authenticate(
	ctx context.Context,
	clientID string,
	username string,
	password string,
) (ClientInfo, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info, ok, err := m.store.Validate(ctx, clientID, username, password)
	if err != nil {
		m.logger.Error("auth validation failed: %s: %v", clientID, err)
		return ClientInfo{}, false, err
	}
	if !ok {
		m.logger.Debug("auth rejected: %s", clientID)
		return ClientInfo{}, false, nil
	}
	return info, true, nil
}

// Get returns client info without authentication.
func (m *Manager) Get(ctx context.Context, clientID string) (ClientInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.store.Get(ctx, clientID)
}

// Remove deletes client credentials.
func (m *Manager) Remove(ctx context.Context, clientID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.store.Remove(ctx, clientID); err != nil {
		return err
	}
	m.logger.Info("removed auth client: %s", clientID)
	return nil
}

// List returns active client identifiers.
func (m *Manager) List(ctx context.Context) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.store.List(ctx)
}

// Stats returns debug information from the store backend.
func (m *Manager) Stats(ctx context.Context) (map[string]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.store.Stats(ctx)
}

// Close releases underlying resources.
func (m *Manager) Close() error {
	var err error

	m.cleanupOnce.Do(func() {
		close(m.cleanupStop)
	})

	m.mu.Lock()
	defer m.mu.Unlock()

	if closeErr := m.store.Close(context.Background()); closeErr != nil {
		err = closeErr
		m.logger.Error("failed closing auth store: %v", closeErr)
	}
	return err
}

// GenerateSessionKeys provides crypto keys for secure channels.
func (m *Manager) GenerateSessionKeys(sessionID string) (*SessionKeys, error) {
	return m.crypto.GenerateSessionKeys(sessionID)
}

// GetSessionKeys loads cached keys by session identifier.
func (m *Manager) GetSessionKeys(sessionID string) (*SessionKeys, error) {
	return m.crypto.GetSessionKeys(sessionID)
}

// RevokeSession revokes keys associated with the session.
func (m *Manager) RevokeSession(sessionID string) error {
	return m.crypto.RevokeSessionKeys(sessionID)
}

// Legacy-friendly helpers ----------------------------------------------------

// RegisterClient stores credentials using background context.
func (m *Manager) RegisterClientLegacy(
	clientID string,
	username string,
	password string,
	metadata map[string]any,
) error {
	info := ClientInfo{
		ClientID: clientID,
		Username: username,
		Password: password,
		Metadata: metadata,
	}
	return m.RegisterClient(context.Background(), info)
}

// AuthenticateClient mirrors the historic API returning pointer for compatibility.
func (m *Manager) AuthenticateClientLegacy(
	clientID string,
	username string,
	password string,
) (bool, *ClientInfo, error) {
	info, ok, err := m.Authenticate(context.Background(), clientID, username, password)
	if err != nil || !ok {
		return ok, nil, err
	}
	return true, &info, nil
}

// GetClientInfoLegacy fetches client info via background context.
func (m *Manager) GetClientInfoLegacy(clientID string) (*ClientInfo, error) {
	info, err := m.Get(context.Background(), clientID)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// RemoveClientLegacy removes stored credentials.
func (m *Manager) RemoveClientLegacy(clientID string) error {
	return m.Remove(context.Background(), clientID)
}

// ListClientsLegacy returns active client identifiers.
func (m *Manager) ListClientsLegacy() ([]string, error) {
	return m.List(context.Background())
}

// CleanupExpiredLegacy forces store cleanup.
func (m *Manager) CleanupExpiredLegacy() error {
	return m.store.CleanupExpired(context.Background())
}

// GetStatsLegacy returns store stats ignoring errors for backward compatibility.
func (m *Manager) GetStatsLegacy() map[string]any {
	stats, err := m.Stats(context.Background())
	if err != nil {
		m.logger.Warn("auth stats unavailable: %v", err)
		return map[string]any{"error": err.Error()}
	}
	return stats
}
