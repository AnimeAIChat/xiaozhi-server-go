package repository

import (
	"context"

	"xiaozhi-server-go/internal/domain/device/aggregate"
)

// DeviceRepository 设备仓库接口
type DeviceRepository interface {
	// Save 保存设备
	Save(ctx context.Context, device *aggregate.Device) error

	// FindByDeviceID 根据设备ID查找设备
	FindByDeviceID(ctx context.Context, deviceID string) (*aggregate.Device, error)

	// FindByID 根据ID查找设备
	FindByID(ctx context.Context, id int) (*aggregate.Device, error)

	// Update 更新设备信息
	Update(ctx context.Context, device *aggregate.Device) error

	// Delete 删除设备
	Delete(ctx context.Context, deviceID string) error

	// ListByUserID 根据用户ID列出设备
	ListByUserID(ctx context.Context, userID int) ([]*aggregate.Device, error)
}

// VerificationCodeRepository 验证码仓库接口
type VerificationCodeRepository interface {
	// Save 保存验证码
	Save(ctx context.Context, code *aggregate.VerificationCode) error

	// FindByCode 根据验证码查找
	FindByCode(ctx context.Context, code string, purpose aggregate.VerificationCodePurpose) (*aggregate.VerificationCode, error)

	// FindByDeviceID 根据设备ID查找验证码
	FindByDeviceID(ctx context.Context, deviceID string, purpose aggregate.VerificationCodePurpose) ([]*aggregate.VerificationCode, error)

	// Delete 删除验证码
	Delete(ctx context.Context, code string, purpose aggregate.VerificationCodePurpose) error

	// DeleteExpired 删除过期验证码
	DeleteExpired(ctx context.Context) error
}