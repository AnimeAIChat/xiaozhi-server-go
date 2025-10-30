package service_test

import (
	"context"
	"testing"
	"time"

	"xiaozhi-server-go/internal/domain/device/aggregate"
	"xiaozhi-server-go/internal/domain/device/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockDeviceRepository 模拟设备仓库
type MockDeviceRepository struct {
	mock.Mock
	devices map[string]*aggregate.Device
}

func NewMockDeviceRepository() *MockDeviceRepository {
	return &MockDeviceRepository{
		devices: make(map[string]*aggregate.Device),
	}
}

func (m *MockDeviceRepository) Save(ctx context.Context, device *aggregate.Device) error {
	args := m.Called(ctx, device)
	m.devices[device.DeviceID] = device
	return args.Error(0)
}

func (m *MockDeviceRepository) Update(ctx context.Context, device *aggregate.Device) error {
	args := m.Called(ctx, device)
	m.devices[device.DeviceID] = device
	return args.Error(0)
}

func (m *MockDeviceRepository) FindByDeviceID(ctx context.Context, deviceID string) (*aggregate.Device, error) {
	args := m.Called(ctx, deviceID)
	return args.Get(0).(*aggregate.Device), args.Error(1)
}

func (m *MockDeviceRepository) FindByID(ctx context.Context, id int) (*aggregate.Device, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(*aggregate.Device), args.Error(1)
}

func (m *MockDeviceRepository) FindByClientID(ctx context.Context, clientID string) (*aggregate.Device, error) {
	args := m.Called(ctx, clientID)
	return args.Get(0).(*aggregate.Device), args.Error(1)
}

func (m *MockDeviceRepository) FindAll(ctx context.Context) ([]*aggregate.Device, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*aggregate.Device), args.Error(1)
}

func (m *MockDeviceRepository) Delete(ctx context.Context, deviceID string) error {
	args := m.Called(ctx, deviceID)
	delete(m.devices, deviceID)
	return args.Error(0)
}

func (m *MockDeviceRepository) ListByUserID(ctx context.Context, userID int) ([]*aggregate.Device, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]*aggregate.Device), args.Error(1)
}

// MockVerificationCodeRepository 模拟验证码仓库
type MockVerificationCodeRepository struct {
	mock.Mock
	codes map[string]*aggregate.VerificationCode
}

func NewMockVerificationCodeRepository() *MockVerificationCodeRepository {
	return &MockVerificationCodeRepository{
		codes: make(map[string]*aggregate.VerificationCode),
	}
}

func (m *MockVerificationCodeRepository) Save(ctx context.Context, code *aggregate.VerificationCode) error {
	args := m.Called(ctx, code)
	m.codes[code.Code] = code
	return args.Error(0)
}

func (m *MockVerificationCodeRepository) FindByCode(ctx context.Context, code string, purpose aggregate.VerificationCodePurpose) (*aggregate.VerificationCode, error) {
	args := m.Called(ctx, code, purpose)
	return args.Get(0).(*aggregate.VerificationCode), args.Error(1)
}

func (m *MockVerificationCodeRepository) FindByDeviceID(ctx context.Context, deviceID string, purpose aggregate.VerificationCodePurpose) ([]*aggregate.VerificationCode, error) {
	args := m.Called(ctx, deviceID, purpose)
	var result []*aggregate.VerificationCode
	for _, vc := range m.codes {
		if vc.DeviceID != nil && *vc.DeviceID == deviceID && vc.Purpose == purpose {
			result = append(result, vc)
		}
	}
	return result, args.Error(2)
}

func (m *MockVerificationCodeRepository) Delete(ctx context.Context, code string, purpose aggregate.VerificationCodePurpose) error {
	args := m.Called(ctx, code, purpose)
	delete(m.codes, code)
	return args.Error(0)
}

func (m *MockVerificationCodeRepository) DeleteExpired(ctx context.Context) error {
	args := m.Called(ctx)
	now := time.Now()
	for code, vc := range m.codes {
		if vc.ExpiresAt.Before(now) {
			delete(m.codes, code)
		}
	}
	return args.Error(0)
}

func TestDeviceService_RegisterDevice_NoActivation(t *testing.T) {
	// 创建模拟仓库
	deviceRepo := NewMockDeviceRepository()
	verificationRepo := NewMockVerificationCodeRepository()

	// 创建服务（不需要激活码）
	service := service.NewDeviceService(deviceRepo, verificationRepo, false, 1)

	// 设置模拟期望
	ctx := context.Background()
	deviceRepo.On("FindByDeviceID", ctx, "test-device-001").Return((*aggregate.Device)(nil), nil)
	deviceRepo.On("Save", ctx, mock.AnythingOfType("*aggregate.Device")).Return(nil)

	// 执行注册
	device, err := service.RegisterDevice(ctx, "test-device-001", "client-001", "Test Device", "1.0.0", "192.168.1.100", "app-info")

	// 验证结果
	assert.NoError(t, err)
	assert.NotNil(t, device)
	assert.Equal(t, "test-device-001", device.DeviceID)
	assert.Equal(t, "client-001", device.ClientID)
	assert.Equal(t, "Test Device", device.Name)
	assert.Equal(t, "1.0.0", device.Version)
	assert.Equal(t, "192.168.1.100", device.LastIP)
	assert.Equal(t, "app-info", device.Application)
	assert.True(t, device.IsActivated()) // 应该自动激活

	// 验证模拟调用
	deviceRepo.AssertExpectations(t)
	verificationRepo.AssertExpectations(t)
}

func TestDeviceService_RegisterDevice_WithActivation(t *testing.T) {
	// 创建模拟仓库
	deviceRepo := NewMockDeviceRepository()
	verificationRepo := NewMockVerificationCodeRepository()

	// 创建服务（需要激活码）
	service := service.NewDeviceService(deviceRepo, verificationRepo, true, 1)

	// 设置模拟期望
	ctx := context.Background()
	deviceRepo.On("FindByDeviceID", ctx, "test-device-002").Return((*aggregate.Device)(nil), nil)
	verificationRepo.On("Save", ctx, mock.AnythingOfType("*aggregate.VerificationCode")).Return(nil)
	deviceRepo.On("Save", ctx, mock.AnythingOfType("*aggregate.Device")).Return(nil)

	// 执行注册
	device, err := service.RegisterDevice(ctx, "test-device-002", "client-002", "Test Device 2", "2.0.0", "192.168.1.101", "app-info-2")

	// 验证结果
	assert.NoError(t, err)
	assert.NotNil(t, device)
	assert.Equal(t, "test-device-002", device.DeviceID)
	assert.Equal(t, "client-002", device.ClientID)
	assert.Equal(t, "Test Device 2", device.Name)
	assert.Equal(t, "2.0.0", device.Version)
	assert.Equal(t, "192.168.1.101", device.LastIP)
	assert.Equal(t, "app-info-2", device.Application)
	assert.False(t, device.IsActivated()) // 不应该自动激活
	assert.NotEmpty(t, device.AuthCode)   // 应该有激活码

	// 验证模拟调用
	deviceRepo.AssertExpectations(t)
	verificationRepo.AssertExpectations(t)
}

func TestDeviceService_ActivateDevice(t *testing.T) {
	// 创建模拟仓库
	deviceRepo := NewMockDeviceRepository()
	verificationRepo := NewMockVerificationCodeRepository()

	// 创建服务
	service := service.NewDeviceService(deviceRepo, verificationRepo, true, 1)

	ctx := context.Background()

	// 创建测试设备和验证码
	device, _ := aggregate.NewDevice("test-device-003", "client-003", "Test Device 3", "3.0.0")
	verificationCode, _ := aggregate.NewVerificationCode(aggregate.CodePurposeDeviceActivation, "test-device-003", 24)

	// 将验证码添加到mock仓库中
	verificationRepo.codes[verificationCode.Code] = verificationCode

	// 设置模拟期望
	deviceRepo.On("FindByDeviceID", ctx, "test-device-003").Return(device, nil)
	verificationRepo.On("FindByCode", ctx, verificationCode.Code, aggregate.CodePurposeDeviceActivation).Return(verificationCode, nil)
	deviceRepo.On("Update", ctx, device).Return(nil)
	verificationRepo.On("Save", ctx, verificationCode).Return(nil)

	// 执行激活
	err := service.ActivateDevice(ctx, "test-device-003", verificationCode.Code)

	// 验证结果
	assert.NoError(t, err)
	assert.True(t, device.IsActivated())

	// 验证模拟调用
	deviceRepo.AssertExpectations(t)
	verificationRepo.AssertExpectations(t)
}