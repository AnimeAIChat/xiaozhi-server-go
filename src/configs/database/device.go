package database

import (
	"time"
	"xiaozhi-server-go/src/models"

	"gorm.io/gorm"
)

// 支持事务的 FindDeviceByID
func FindDeviceByID(tx *gorm.DB, id string) (*models.Device, error) {
	var device models.Device
	if err := tx.Where("device_id = ?", id).First(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

func FindDeletedDeviceByID(tx *gorm.DB, id string) (*models.Device, error) {
	var device models.Device
	if err := tx.Unscoped().Where("device_id = ?", id).First(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

// 硬删除设备
func HardDeleteDevice(tx *gorm.DB, id string) error {
	if err := tx.Unscoped().Where("device_id = ?", id).Delete(&models.Device{}).Error; err != nil {
		return err
	}
	return nil
}

// 支持事务的 FindDeviceByIDAndUser
func FindDeviceByIDAndUser(tx *gorm.DB, id string, userID uint) (*models.Device, error) {
	var device models.Device
	if err := tx.Where("device_id = ? AND user_id = ?", id, userID).First(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

// 支持事务的 ListDevicesByAgent
func ListDevicesByAgent(tx *gorm.DB, agentID uint) ([]models.Device, error) {
	var devices []models.Device
	if err := tx.Where("agent_id = ?", agentID).Find(&devices).Error; err != nil {
		return nil, err
	}
	return devices, nil
}

// 支持事务的 ListDevicesByUser
func ListDevicesByUser(tx *gorm.DB, userID uint) ([]models.Device, error) {
	var devices []models.Device
	if err := tx.Where("user_id = ?", userID).Find(&devices).Error; err != nil {
		return nil, err
	}
	return devices, nil
}

// 支持事务的 AddDevice
func AddDevice(tx *gorm.DB, device *models.Device) error {
	if err := tx.Create(device).Error; err != nil {
		return err
	}
	return nil
}

// 支持事务的 UpdateDevice
func UpdateDevice(tx *gorm.DB, device *models.Device) error {
	if err := tx.Save(device).Error; err != nil {
		return err
	}
	return nil
}

func DeleteDevice(tx *gorm.DB, deviceID string) error {
	// 先将设备记录的 agent_id 置为 NULL（避免软删除后仍引用 agent）
	if err := tx.Model(&models.Device{}).Where("device_id = ?", deviceID).Update("agent_id", nil).Error; err != nil {
		return err
	}
	// 软删除设备
	if err := tx.Where("device_id = ?", deviceID).Delete(&models.Device{}).Error; err != nil {
		return err
	}
	return nil
}

func UpdateDeviceConversationID(tx *gorm.DB, deviceID string, conversationID string) error {
	// 更新设备的会话ID, 同时更新last_active_time_v2
	updates := map[string]interface{}{
		"conversationid":      conversationID,
		"last_active_time_v2": time.Now(),
	}
	return tx.Model(&models.Device{}).Where("device_id = ?", deviceID).
		UpdateColumns(updates).Error
}
