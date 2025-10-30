package store

import (
	"errors"
)

type DatabaseAuthStore struct {
	expiry int
}

func NewDatabaseAuthStore(expiryHr int) *DatabaseAuthStore {
	return &DatabaseAuthStore{
		expiry: expiryHr,
	}
}

func (s *DatabaseAuthStore) StoreAuth(
	clientID, username, password string,
	metadata map[string]interface{},
) error {
	return errors.New("database functionality removed")
}

func (s *DatabaseAuthStore) ValidateAuth(
	clientID, username, password string,
) (bool, *ClientInfo, error) {
	return false, nil, errors.New("database functionality removed")
}

func (s *DatabaseAuthStore) GetClientInfo(clientID string) (*ClientInfo, error) {
	return nil, errors.New("database functionality removed")
}

func (s *DatabaseAuthStore) RemoveAuth(clientID string) error {
	return errors.New("database functionality removed")
}

func (s *DatabaseAuthStore) ListClients() ([]string, error) {
	return nil, errors.New("database functionality removed")
}

func (s *DatabaseAuthStore) CleanupExpired() error {
	return errors.New("database functionality removed")
}

func (s *DatabaseAuthStore) Close() error {
	return nil
}
