package database

import (
	"strings"
	"time"
	"xiaozhi-server-go/src/models"

	"gorm.io/gorm"
)

// NormalizeMACAddress converts common MAC address formats to AA:BB:CC:DD:EE:FF.
// An empty string means the value is not a valid MAC address.
func NormalizeMACAddress(value string) string {
	compact := strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(strings.TrimSpace(value))
	if len(compact) != 12 {
		return ""
	}
	for _, char := range compact {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return ""
		}
	}
	compact = strings.ToUpper(compact)
	return strings.Join([]string{compact[0:2], compact[2:4], compact[4:6], compact[6:8], compact[8:10], compact[10:12]}, ":")
}

// 支持事务的 FindDeviceByID
func FindDeviceByID(tx *gorm.DB, id string) (*models.Device, error) {
	var device models.Device
	if err := tx.Where("device_id = ?", id).First(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

// FindDeviceByMAC finds a device by its reported physical MAC. Older records
// that only stored the MAC in device_id remain compatible.
func FindDeviceByMAC(tx *gorm.DB, macAddress string) (*models.Device, error) {
	normalized := NormalizeMACAddress(macAddress)
	if normalized == "" {
		return nil, gorm.ErrRecordNotFound
	}

	var device models.Device
	if err := tx.Where("mac_address = ?", normalized).First(&device).Error; err == nil {
		return &device, nil
	} else if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	compact := strings.ToLower(strings.ReplaceAll(normalized, ":", ""))
	if err := tx.Where("LOWER(REPLACE(REPLACE(REPLACE(device_id, ':', ''), '-', ''), '.', '')) = ?", compact).First(&device).Error; err != nil {
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

func ListDevicesByAgentAndUser(tx *gorm.DB, agentID, userID uint) ([]models.Device, error) {
	var devices []models.Device
	if err := tx.Where("agent_id = ? AND user_id = ?", agentID, userID).Find(&devices).Error; err != nil {
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
