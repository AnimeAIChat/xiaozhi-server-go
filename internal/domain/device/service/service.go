package service

import (
	"context"
	"time"

	"xiaozhi-server-go/internal/domain/device/aggregate"
	"xiaozhi-server-go/internal/domain/device/repository"
	"xiaozhi-server-go/internal/platform/errors"
)

// DeviceService 设备领域服务
type DeviceService struct {
	deviceRepo        repository.DeviceRepository
	verificationRepo  repository.VerificationCodeRepository
	requireActivation bool
	defaultAdminUserID int
}

// NewDeviceService 创建设备服务
func NewDeviceService(
	deviceRepo repository.DeviceRepository,
	verificationRepo repository.VerificationCodeRepository,
	requireActivation bool,
	defaultAdminUserID int,
) *DeviceService {
	return &DeviceService{
		deviceRepo:        deviceRepo,
		verificationRepo:  verificationRepo,
		requireActivation: requireActivation,
		defaultAdminUserID: defaultAdminUserID,
	}
}

// RegisterDevice 注册设备
func (s *DeviceService) RegisterDevice(
	ctx context.Context,
	deviceID, clientID, name, version, ip string,
	appInfo string,
) (*aggregate.Device, error) {
	// 检查设备是否已存在
	existingDevice, err := s.deviceRepo.FindByDeviceID(ctx, deviceID)
	if err != nil {
		return nil, errors.Wrap(errors.KindDomain, "device.register", "failed to check existing device", err)
	}

	if existingDevice != nil {
		// 设备已存在，更新信息
		existingDevice.UpdateActivity(ip, appInfo)
		return existingDevice, s.deviceRepo.Update(ctx, existingDevice)
	}

	// 创建新设备
	device, err := aggregate.NewDevice(deviceID, clientID, name, version)
	if err != nil {
		return nil, errors.Wrap(errors.KindDomain, "device.register", "failed to create device", err)
	}

	// 设置初始信息
	device.LastIP = ip
	device.Application = appInfo

	// 根据配置决定是否需要激活码
	if s.requireActivation {
		// 生成激活码
		verificationCode, err := aggregate.NewVerificationCode(
			aggregate.CodePurposeDeviceActivation,
			deviceID,
			24, // 24小时有效期
		)
		if err != nil {
			return nil, errors.Wrap(errors.KindDomain, "device.register", "failed to generate verification code", err)
		}

		device.SetAuthCode(verificationCode.Code)

		// 保存验证码
		if err := s.verificationRepo.Save(ctx, verificationCode); err != nil {
			return nil, errors.Wrap(errors.KindDomain, "device.register", "failed to save verification code", err)
		}
	} else {
		// 直接绑定管理员用户
		if err := device.Approve(s.defaultAdminUserID); err != nil {
			return nil, errors.Wrap(errors.KindDomain, "device.register", "failed to approve device", err)
		}
	}

	// 保存设备
	if err := s.deviceRepo.Save(ctx, device); err != nil {
		return nil, errors.Wrap(errors.KindDomain, "device.register", "failed to save device", err)
	}

	return device, nil
}

// ActivateDevice 激活设备
func (s *DeviceService) ActivateDevice(
	ctx context.Context,
	deviceID, authCode string,
) error {
	// 查找设备
	device, err := s.deviceRepo.FindByDeviceID(ctx, deviceID)
	if err != nil {
		return errors.Wrap(errors.KindDomain, "device.activate", "device not found", err)
	}

	if device.IsActivated() {
		return errors.New(errors.KindDomain, "device.activate", "device already activated")
	}

	// 验证激活码
	verificationCode, err := s.verificationRepo.FindByCode(ctx, authCode, aggregate.CodePurposeDeviceActivation)
	if err != nil {
		return errors.Wrap(errors.KindDomain, "device.activate", "invalid verification code", err)
	}

	if !verificationCode.IsValid() {
		return errors.New(errors.KindDomain, "device.activate", "verification code expired or used")
	}

	if !verificationCode.MatchesDevice(deviceID) {
		return errors.New(errors.KindDomain, "device.activate", "verification code does not match device")
	}

	// 激活设备
	if err := device.Approve(s.defaultAdminUserID); err != nil {
		return errors.Wrap(errors.KindDomain, "device.activate", "failed to approve device", err)
	}

	// 使用验证码
	if err := verificationCode.Use(); err != nil {
		return errors.Wrap(errors.KindDomain, "device.activate", "failed to use verification code", err)
	}

	// 保存更改
	if err := s.deviceRepo.Update(ctx, device); err != nil {
		return errors.Wrap(errors.KindDomain, "device.activate", "failed to update device", err)
	}

	if err := s.verificationRepo.Save(ctx, verificationCode); err != nil {
		return errors.Wrap(errors.KindDomain, "device.activate", "failed to update verification code", err)
	}

	return nil
}

// GetDevice 获取设备信息
func (s *DeviceService) GetDevice(ctx context.Context, deviceID string) (*aggregate.Device, error) {
	device, err := s.deviceRepo.FindByDeviceID(ctx, deviceID)
	if err != nil {
		return nil, errors.Wrap(errors.KindDomain, "device.get", "failed to find device", err)
	}
	return device, nil
}

// UpdateDeviceActivity 更新设备活动
func (s *DeviceService) UpdateDeviceActivity(ctx context.Context, deviceID, ip, appInfo string) error {
	device, err := s.deviceRepo.FindByDeviceID(ctx, deviceID)
	if err != nil {
		return errors.Wrap(errors.KindDomain, "device.update_activity", "device not found", err)
	}

	device.UpdateActivity(ip, appInfo)
	return s.deviceRepo.Update(ctx, device)
}

// SetSessionEnd 设置会话结束时间
func (s *DeviceService) SetSessionEnd(ctx context.Context, deviceID string) error {
	device, err := s.deviceRepo.FindByDeviceID(ctx, deviceID)
	if err != nil {
		return errors.Wrap(errors.KindDomain, "device.set_session_end", "device not found", err)
	}

	device.SetLastSessionEnd(time.Now())
	return s.deviceRepo.Update(ctx, device)
}