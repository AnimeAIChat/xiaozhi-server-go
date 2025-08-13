package store

import (
	"encoding/json"
	"time"
	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/models"

	"gorm.io/gorm"
)

type DatabaseAuthStore struct {
	db     *gorm.DB
	expiry int
}

func NewDatabaseAuthStore(expiryHr int) *DatabaseAuthStore {
	return &DatabaseAuthStore{
		db:     database.DB,
		expiry: expiryHr,
	}
}

func (s *DatabaseAuthStore) StoreAuth(
	clientID, username, password string,
	metadata map[string]interface{},
) error {
	now := time.Now()
	var expiresAt *time.Time
	if s.expiry > 0 {
		exp := now.Add(time.Duration(s.expiry) * time.Hour)
		expiresAt = &exp
	}
	metaJson, _ := json.Marshal(metadata)
	authClient := &models.AuthClient{
		ClientID:  clientID,
		Username:  username,
		Password:  password,
		CreatedAt: now,
		ExpiresAt: expiresAt,
		Metadata:  metaJson,
	}
	// Delete old record if exists
	if err := s.db.Where("client_id = ?", clientID).Delete(&models.AuthClient{}).Error; err != nil {
		return err
	}
	return s.db.Create(authClient).Error
}

func (s *DatabaseAuthStore) ValidateAuth(
	clientID, username, password string,
) (bool, *ClientInfo, error) {
	var auth models.AuthClient
	err := s.db.Where("client_id = ?", clientID).First(&auth).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil, nil
		}
		return false, nil, err
	}
	if auth.Username != username || auth.Password != password {
		return false, nil, nil
	}
	clientInfo := &ClientInfo{
		ClientID:  auth.ClientID,
		Username:  auth.Username,
		Password:  auth.Password,
		IP:        auth.IP,
		DeviceID:  auth.DeviceID,
		CreatedAt: auth.CreatedAt,
		ExpiresAt: auth.ExpiresAt,
	}
	if len(auth.Metadata) > 0 {
		var meta map[string]interface{}
		_ = json.Unmarshal(auth.Metadata, &meta)
		clientInfo.Metadata = meta
	}
	return true, clientInfo, nil
}

func (s *DatabaseAuthStore) GetClientInfo(clientID string) (*ClientInfo, error) {
	var auth models.AuthClient
	err := s.db.Where("client_id = ?", clientID).First(&auth).Error
	if err != nil {
		return nil, err
	}
	clientInfo := &ClientInfo{
		ClientID:  auth.ClientID,
		Username:  auth.Username,
		Password:  auth.Password,
		IP:        auth.IP,
		DeviceID:  auth.DeviceID,
		CreatedAt: auth.CreatedAt,
		ExpiresAt: auth.ExpiresAt,
	}
	if len(auth.Metadata) > 0 {
		var meta map[string]interface{}
		_ = json.Unmarshal(auth.Metadata, &meta)
		clientInfo.Metadata = meta
	}
	return clientInfo, nil
}

func (s *DatabaseAuthStore) RemoveAuth(clientID string) error {
	return s.db.Where("client_id = ?", clientID).Delete(&models.AuthClient{}).Error
}

func (s *DatabaseAuthStore) ListClients() ([]string, error) {
	var clients []models.AuthClient
	err := s.db.Select("client_id").Find(&clients).Error
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, c := range clients {
		ids = append(ids, c.ClientID)
	}
	return ids, nil
}

func (s *DatabaseAuthStore) CleanupExpired() error {
	now := time.Now()
	return s.db.Where("expires_at IS NOT NULL AND expires_at < ?", now).
		Delete(&models.AuthClient{}).
		Error
}

func (s *DatabaseAuthStore) Close() error {
	// GORM does not require explicit close for global DB, so do nothing
	return nil
}
