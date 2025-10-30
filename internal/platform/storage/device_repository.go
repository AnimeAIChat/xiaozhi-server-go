package storage

import (
	"context"

	"gorm.io/gorm"

	"xiaozhi-server-go/internal/domain/device/aggregate"
	"xiaozhi-server-go/internal/domain/device/repository"
	"xiaozhi-server-go/internal/platform/errors"
)

// deviceRepository 设备仓库实现
type deviceRepository struct {
	db *gorm.DB
}

// NewDeviceRepository 创建设备仓库实例
func NewDeviceRepository(db *gorm.DB) repository.DeviceRepository {
	return &deviceRepository{
		db: db,
	}
}

// Save 保存设备
func (r *deviceRepository) Save(ctx context.Context, device *aggregate.Device) error {
	model := r.toModel(device)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return errors.Wrap(errors.KindStorage, "device.save", "failed to save device", err)
	}
	return nil
}

// Update 更新设备
func (r *deviceRepository) Update(ctx context.Context, device *aggregate.Device) error {
	model := r.toModel(device)
	if err := r.db.WithContext(ctx).Save(model).Error; err != nil {
		return errors.Wrap(errors.KindStorage, "device.update", "failed to update device", err)
	}
	return nil
}

// FindByDeviceID 根据设备ID查找设备
func (r *deviceRepository) FindByDeviceID(ctx context.Context, deviceID string) (*aggregate.Device, error) {
	var model Device
	if err := r.db.WithContext(ctx).Where("device_id = ?", deviceID).First(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // 设备不存在
		}
		return nil, errors.Wrap(errors.KindStorage, "device.find_by_device_id", "failed to find device", err)
	}
	return r.fromModel(&model), nil
}

// FindByID 根据ID查找设备
func (r *deviceRepository) FindByID(ctx context.Context, id int) (*aggregate.Device, error) {
	var model Device
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // 设备不存在
		}
		return nil, errors.Wrap(errors.KindStorage, "device.find_by_id", "failed to find device", err)
	}
	return r.fromModel(&model), nil
}

// ListByUserID 根据用户ID列出设备
func (r *deviceRepository) ListByUserID(ctx context.Context, userID int) ([]*aggregate.Device, error) {
	var models []Device
	if err := r.db.WithContext(ctx).Where("user_id = ?", uint(userID)).Find(&models).Error; err != nil {
		return nil, errors.Wrap(errors.KindStorage, "device.list_by_user_id", "failed to find devices", err)
	}

	devices := make([]*aggregate.Device, len(models))
	for i, model := range models {
		devices[i] = r.fromModel(&model)
	}
	return devices, nil
}

// FindAll 获取所有设备
func (r *deviceRepository) FindAll(ctx context.Context) ([]*aggregate.Device, error) {
	var models []Device
	if err := r.db.WithContext(ctx).Find(&models).Error; err != nil {
		return nil, errors.Wrap(errors.KindStorage, "device.find_all", "failed to find devices", err)
	}

	devices := make([]*aggregate.Device, len(models))
	for i, model := range models {
		devices[i] = r.fromModel(&model)
	}
	return devices, nil
}

// Delete 删除设备
func (r *deviceRepository) Delete(ctx context.Context, deviceID string) error {
	if err := r.db.WithContext(ctx).Where("device_id = ?", deviceID).Delete(&Device{}).Error; err != nil {
		return errors.Wrap(errors.KindStorage, "device.delete", "failed to delete device", err)
	}
	return nil
}

// toModel 将领域对象转换为存储模型
func (r *deviceRepository) toModel(device *aggregate.Device) *Device {
	model := &Device{
		ID:               uint(device.ID),
		Name:             device.Name,
		DeviceID:         device.DeviceID,
		ClientID:         device.ClientID,
		Version:          device.Version,
		RegisterTime:     device.RegisterTime.Unix(),
		RegisterTimeV2:   device.RegisterTime,
		LastActiveTime:   device.LastActiveTime.Unix(),
		LastActiveTimeV2: device.LastActiveTime,
		Online:           device.Online,
		AuthCode:         device.AuthCode,
		AuthStatus:       string(device.AuthStatus),
		BoardType:        device.BoardType,
		ChipModelName:    device.ChipModelName,
		Channel:          device.Channel,
		SSID:             device.SSID,
		Application:      device.Application,
		Language:         device.Language,
		DeviceCode:       device.DeviceCode,
		Conversationid:   device.ConversationID,
		Mode:             device.Mode,
		LastIP:           device.LastIP,
		Stats:            device.Stats,
		TotalTokens:      device.TotalTokens,
		UsedTokens:       device.UsedTokens,
		LastSessionEndAt: device.LastSessionEndAt,
	}

	if device.UserID != nil {
		userID := uint(*device.UserID)
		model.UserID = &userID
	}
	if device.AgentID != nil {
		agentID := uint(*device.AgentID)
		model.AgentID = &agentID
	}

	return model
}

// fromModel 将存储模型转换为领域对象
func (r *deviceRepository) fromModel(model *Device) *aggregate.Device {
	device := &aggregate.Device{
		ID:               int(model.ID),
		Name:             model.Name,
		DeviceID:         model.DeviceID,
		ClientID:         model.ClientID,
		Version:          model.Version,
		RegisterTime:     model.RegisterTimeV2,
		LastActiveTime:   model.LastActiveTimeV2,
		Online:           model.Online,
		AuthCode:         model.AuthCode,
		AuthStatus:       aggregate.DeviceStatus(model.AuthStatus),
		BoardType:        model.BoardType,
		ChipModelName:    model.ChipModelName,
		Channel:          model.Channel,
		SSID:             model.SSID,
		Application:      model.Application,
		Language:         model.Language,
		DeviceCode:       model.DeviceCode,
		ConversationID:   model.Conversationid,
		Mode:             model.Mode,
		LastIP:           model.LastIP,
		Stats:            model.Stats,
		TotalTokens:      model.TotalTokens,
		UsedTokens:       model.UsedTokens,
		LastSessionEndAt: model.LastSessionEndAt,
	}

	if model.UserID != nil {
		userID := int(*model.UserID)
		device.UserID = &userID
	}
	if model.AgentID != nil {
		agentID := int(*model.AgentID)
		device.AgentID = &agentID
	}

	return device
}